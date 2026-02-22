package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

const (
	timeout    = 3 * time.Second
	maxRetries = 3
)

func Connect(dsn string) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var err error
	for i := 0; i < maxRetries; i++ {

		pool, err = pgxpool.New(context.Background(), dsn)

		if err != nil {
			fmt.Printf("DB connect attempt %d failed: %v\n", i+1, err)
			time.Sleep(timeout)
			continue
		}

		err = pool.Ping(context.Background())
		if err != nil {
			fmt.Printf("DB ping attempt %d failed: %v\n", i+1, err)
			time.Sleep(timeout)
			continue
		}

		fmt.Println("DB connected")
		return pool, nil
	}

	return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, err)
}

func Migrate(pool *pgxpool.Pool) error {
	dsn := pool.Config().ConnString()

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("migrate: open sql.DB: %w", err)
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: set dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "./migrations"); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}

	return nil
}
