package logger

import (
	"fmt"
	"io"
	"net/http"
	"observeddb-go-api/cfg"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func MustInit(logConfig cfg.LogConfig, debug bool) {
	// Log level
	zerLogLevel, err := logLevel(logConfig.Level, debug)
	if err != nil {
		panic(fmt.Sprintf("Error setting log level: %v", err))
	}

	// Writer
	writer, err := writer(logConfig, debug)
	if err != nil {
		panic(fmt.Sprintf("Error setting log writer: %v", err))
	}

	// Setup Logger
	zerolog.SetGlobalLevel(zerLogLevel)
	log.Logger = zerolog.New(writer).With().Timestamp().Logger()
}

func LogServerError(c *gin.Context, err error) {
	log.Error().
		Str("endpoint", c.FullPath()).
		Str("method", c.Request.Method).
		Int("status", http.StatusInternalServerError).
		Err(err).
		Msg("Internal server error")
}

func LogForbiddenError(c *gin.Context, msg string) {
	log.Info().
		Str("endpoint", c.FullPath()).
		Str("method", c.Request.Method).
		Str("ip", c.ClientIP()).
		Str("user_agent", c.GetHeader("User-Agent")).
		Int("status", http.StatusUnauthorized).
		Msg(msg)
}

func LogUnauthorizedError(c *gin.Context) {
	log.Info().
		Str("endpoint", c.FullPath()).
		Str("method", c.Request.Method).
		Str("ip", c.ClientIP()).
		Str("user_agent", c.GetHeader("User-Agent")).
		Int("status", http.StatusUnauthorized).
		Msg("Unauthorized")
}

func writer(logConfig cfg.LogConfig, debug bool) (io.Writer, error) {
	logFile, err := initLogFile(logConfig.FileName, logConfig.MaxSize, logConfig.MaxBackups)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize log file: %w", err)
	}

	// Setup Logger
	var writers io.Writer
	if debug {
		consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
		writers = io.MultiWriter(logFile, consoleWriter)
	} else {
		writers = logFile
	}

	return writers, nil
}

func logLevel(logLevel string, debug bool) (zerolog.Level, error) {
	zerLogLevel, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		return zerolog.DebugLevel, fmt.Errorf("cannot parse log level: %w", err)
	}
	if debug {
		zerLogLevel = zerolog.DebugLevel
	}
	return zerLogLevel, nil
}

func initLogFile(fileName string, maxSize int, maxBackups int) (*lumberjack.Logger, error) {
	dirName := filepath.Dir(fileName)
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		return nil, fmt.Errorf("log file directory does not exist: %w", err)
	}

	logger := &lumberjack.Logger{
		Filename:   fileName,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		LocalTime:  false, // Use UTC time
		Compress:   true,
	}

	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		if _, err = os.Create(fileName); err != nil {
			return nil, fmt.Errorf("cannot create log file: %w", err)
		}
	}

	return logger, nil
}
