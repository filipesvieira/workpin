package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/filipesvieira/workpin/services/api/internal/auth"
	"github.com/filipesvieira/workpin/services/api/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		logger.Error("database connection configuration failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	accessTTL := durationFromEnv("AUTH_ACCESS_TTL", 12*time.Hour, logger)
	refreshTTL := durationFromEnv("AUTH_REFRESH_TTL", 30*24*time.Hour, logger)
	if refreshTTL <= accessTTL {
		logger.Error("AUTH_REFRESH_TTL must exceed AUTH_ACCESS_TTL")
		os.Exit(1)
	}
	service := auth.NewService(pool, auth.NewMockSMSProvider(logger), accessTTL, refreshTTL)
	srv := &http.Server{Addr: addr, Handler: server.New(logger, service, pool, os.Getenv("AUTH_COOKIE_SECURE") == "true"), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { logger.Info("api starting", "address", addr); done <- srv.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			logger.Error("shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}

func durationFromEnv(name string, fallback time.Duration, logger *slog.Logger) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		logger.Error("invalid duration environment variable", "name", name)
		os.Exit(1)
	}
	return parsed
}
