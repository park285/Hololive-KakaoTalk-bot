package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type PostgresService struct {
	db     *sql.DB
	logger *zap.Logger
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func NewPostgresService(cfg PostgresConfig, logger *zap.Logger) (*PostgresService, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	logger.Info("PostgreSQL connected",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database),
	)

	return &PostgresService{
		db:     db,
		logger: logger,
	}, nil
}

func (ps *PostgresService) GetDB() *sql.DB {
	return ps.db
}

func (ps *PostgresService) Close() error {
	if ps.db != nil {
		return ps.db.Close()
	}
	return nil
}

func (ps *PostgresService) Ping(ctx context.Context) error {
	return ps.db.PingContext(ctx)
}
