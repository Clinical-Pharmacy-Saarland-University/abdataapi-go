package handle

import (
	"observeddb-go-api/cfg"

	"github.com/jmoiron/sqlx"
	"gorm.io/gorm"
)

// Central struct to hold all the configurations and database connection pool.
type ResourceHandle struct {
	MetaCfg  cfg.MetaConfig
	AuthCfg  cfg.AuthTokenConfig
	ResetCfg cfg.ResetTokenConfig
	Limits   cfg.LimitsConfig
	Gorm     *gorm.DB
	SQLX     *sqlx.DB
}

func NewResourceHandle(
	metaCfg cfg.MetaConfig,
	authCfg cfg.AuthTokenConfig,
	resetCfg cfg.ResetTokenConfig,
	limits cfg.LimitsConfig,
	gorm *gorm.DB,
	sqlx *sqlx.DB,
) *ResourceHandle {
	return &ResourceHandle{
		MetaCfg:  metaCfg,
		AuthCfg:  authCfg,
		ResetCfg: resetCfg,
		Limits:   limits,
		Gorm:     gorm,
		SQLX:     sqlx,
	}
}
