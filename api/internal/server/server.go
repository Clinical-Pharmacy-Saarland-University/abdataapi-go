package server

import (
	"context"
	"fmt"
	"net/http"
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/database"
	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/middleware"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type Server struct {
	Engine       *gin.Engine
	ServerConfig cfg.ServerConfig
}

func New(config *cfg.APIConfig, debug bool) (*Server, error) {
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// create database
	gorm, sqlx, err := database.New(config.Database)
	if err != nil {
		return nil, fmt.Errorf("cannot create database: %w", err)
	}

	// migrate database
	// if err = database.Migrate(db); err != nil {
	// 	return nil, fmt.Errorf("cannot migrate database: %w", err)
	// }

	// setup handler
	resourceHandle := handle.NewResourceHandle(
		config.Meta,
		config.AuthToken,
		config.ResetToken,
		config.Limits,
		gorm,
		sqlx)

	// setup router
	r := gin.New()

	// trusted proxies
	trusedProxies := parseTrustedProxies(config.Server.TrustedProxies)
	if err = r.SetTrustedProxies(trusedProxies); err != nil {
		return nil, fmt.Errorf("cannot set trusted proxies: %w", err)
	}

	// middleware
	r.Use(gin.CustomRecovery(middleware.RecoveryHandler))
	if debug {
		r.Use(gin.Logger())
	}

	// routes
	registerRoutes(r, resourceHandle)

	// server
	srv := &Server{
		Engine:       r,
		ServerConfig: config.Server,
	}

	return srv, nil
}

func (r *Server) Run() {
	srv := &http.Server{
		Addr:         r.ServerConfig.Address,
		Handler:      r.Engine,
		ReadTimeout:  r.ServerConfig.ReadWriteTimeout,
		WriteTimeout: r.ServerConfig.ReadWriteTimeout,
		IdleTimeout:  r.ServerConfig.IdleTimeout,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		_ = srv.ListenAndServe()
	}()
	log.Info().Msgf("Server started on %s", r.ServerConfig.Address)
	<-quit
	log.Info().Msg("Server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
}

func registerRoutes(r *gin.Engine, resourceHandle *handle.ResourceHandle) {
	api := r.Group("/api")

	//RegisterSysRoutes(api, resourceHandle)
	//RegisterUserRoutes(api, resourceHandle)
	//RegisterAdminRoutes(api, resourceHandle)
	RegisterFormulationRoutes(api, resourceHandle)
	RegisterInteractionRoutes(api, resourceHandle)
}

func parseTrustedProxies(proxies string) []string {
	if proxies == "" {
		return nil
	}
	return strings.Split(proxies, ",")
}
