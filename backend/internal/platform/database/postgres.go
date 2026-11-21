package database

import (
	"fmt"
	"time"

	"github.com/mmtaee/ocserv-dashboard/backend/config"
	"github.com/mmtaee/ocserv-dashboard/backend/internal/platform/logging"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	PostgresDB *gorm.DB
)

func ConnectPostgres() error {
	cfg := config.Get()
	db, err := connectPostgres(cfg.DB)
	if err != nil {
		return fmt.Errorf("postgres connection: %w", err)
	}

	finalizePostgres(db, cfg)
	return nil
}

func connectPostgres(cfg config.PostgresConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		cfg.Host, cfg.User, cfg.Password, cfg.DBName, cfg.Port, cfg.SSLMode,
	)

	logger.Info("Connecting Postgres [%s:%s/%s]", cfg.Host, cfg.Port, cfg.DBName)

	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func finalizePostgres(db *gorm.DB, cfg *config.Config) {
	sqlDB, _ := db.DB()

	maxOpen, maxIdle := 20, 10
	if cfg.AgentNode {
		maxOpen, maxIdle = 4, 2
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if cfg.Debug {
		db = db.Debug()
	}

	PostgresDB = db

	logger.Info("Postgres connected successfully")
}

func GetPostgres() *gorm.DB {
	return PostgresDB
}

func ClosePostgres() {
	if PostgresDB != nil {
		if db, _ := PostgresDB.DB(); db != nil {
			_ = db.Close()
		}
	}

	logger.Info("Postgres databases closed")
}

func Connect() error {
	return ConnectPostgres()
}

func GetConnection() *gorm.DB {
	return PostgresDB
}

func Close() {
	ClosePostgres()
}
