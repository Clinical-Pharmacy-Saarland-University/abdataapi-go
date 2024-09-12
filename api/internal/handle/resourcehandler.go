package handle

import (
	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/responder"
	"observeddb-go-api/internal/utils/helper"

	"github.com/jmoiron/sqlx"
	"gorm.io/gorm"
)

// Central struct to hold all the configurations and database connection pool.
type ResourceHandle struct {
	ServerCfg cfg.ServerConfig
	MetaCfg   cfg.MetaConfig
	AuthCfg   cfg.AuthTokenConfig
	ResetCfg  cfg.ResetTokenConfig
	Limits    cfg.LimitsConfig
	Mailer    *responder.Mailer
	Gorm      *gorm.DB
	SQLX      *sqlx.DB
	DebugMode bool
}

func NewResourceHandle(
	cfg *cfg.APIConfig,
	gorm *gorm.DB,
	sqlx *sqlx.DB,
	mailer *responder.Mailer,
	debug bool,
) *ResourceHandle {
	res := &ResourceHandle{
		ServerCfg: cfg.Server,
		MetaCfg:   cfg.Meta,
		AuthCfg:   cfg.AuthToken,
		ResetCfg:  cfg.ResetToken,
		Limits:    cfg.Limits,
		Mailer:    mailer,
		Gorm:      gorm,
		SQLX:      sqlx,
		DebugMode: debug,
	}

	res.MetaCfg.URL = helper.RemoveTrailingSlash(res.MetaCfg.URL)
	res.MetaCfg.Group = helper.RemoveTrailingSlash(res.MetaCfg.Group)
	res.MetaCfg.Group = helper.AddLeadingSlash(res.MetaCfg.Group)

	return res
}
