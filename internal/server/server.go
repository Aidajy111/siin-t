package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Aidajy111/siin.git/internal/config"
	"github.com/rs/zerolog"
)

func Run(ctx context.Context, cfg config.HTTPConfig, router http.Handler, logger zerolog.Logger) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          log.New(errorWriter{logger: logger}, "", 0),
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	logger.Info().Str("addr", cfg.Addr).Msg("starting HTTP server")
	select {
	case err := <-serverErr:
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		logger.Info().Msg("stopping HTTP server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

type errorWriter struct {
	logger zerolog.Logger
}

func (w errorWriter) Write(data []byte) (int, error) {
	w.logger.Error().Str("component", "http_server").Msg(strings.TrimSpace(string(data)))
	return len(data), nil
}
