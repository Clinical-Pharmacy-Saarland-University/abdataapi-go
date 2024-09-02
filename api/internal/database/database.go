package database

import (
	"fmt"
	"observeddb-go-api/cfg"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func New(dbConfig cfg.DatabaseConfig) (*gorm.DB, *sqlx.DB, error) {
	dsn := fmt.Sprintf("@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbConfig.Host, dbConfig.DBName)

	if dbConfig.Password == "" {
		dsn = dbConfig.Username + dsn
	} else {
		dsn = fmt.Sprintf("%s:%s", dbConfig.Username, dbConfig.Password) + dsn
	}

	gorm, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open database %s: %w", dbConfig.DBName, err)
	}

	db, err := gorm.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get database %s: %w", dbConfig.DBName, err)
	}

	db.SetMaxOpenConns(dbConfig.MaxOpenConns)
	db.SetMaxIdleConns(dbConfig.MaxIdleConns)
	db.SetConnMaxLifetime(dbConfig.MaxConnLifetime)

	if err = db.Ping(); err != nil {
		return nil, nil, fmt.Errorf("cannot ping database %s: %w", dbConfig.DBName, err)
	}

	sqlxDB := sqlx.NewDb(db, "mysql")

	return gorm, sqlxDB, nil
}
