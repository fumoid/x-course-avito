package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fumoid/x-course-avito/internal/config"
	"github.com/fumoid/x-course-avito/internal/container"
)

func main() {
	if err := run(); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := container.New(ctx, cfg)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		app.Logger.InfoContext(ctx, "trip-service listening", "addr", cfg.HTTPAddr)
		errCh <- app.Server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = app.Close(shutdownCtx)
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		app.Logger.Info("shutdown signal received", "timeout", cfg.ShutdownTimeout.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		var shutdownErr error
		if err := app.Server.Shutdown(shutdownCtx); err != nil {
			app.Logger.ErrorContext(shutdownCtx, "forced shutdown after timeout", "error", err)
			shutdownErr = fmt.Errorf("graceful HTTP shutdown: %w", err)
			if closeErr := app.Server.Close(); closeErr != nil {
				shutdownErr = errors.Join(shutdownErr, fmt.Errorf("force close HTTP server: %w", closeErr))
			}
		}
		if err := app.Close(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
		app.Logger.Info("trip-service stopped")
		return shutdownErr
	}
}
