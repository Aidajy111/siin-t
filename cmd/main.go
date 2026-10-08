package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Aidajy111/siin.git/internal/config"
	"github.com/Aidajy111/siin.git/internal/database"
	"github.com/Aidajy111/siin.git/internal/handler"
	"github.com/Aidajy111/siin.git/internal/repository/postgres"
	"github.com/Aidajy111/siin.git/internal/server"
	"github.com/Aidajy111/siin.git/internal/service"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("service", "siin").Logger()
	if err := run(logger); err != nil {
		logger.Error().Err(err).Msg("application stopped")
		os.Exit(1)
	}
	logger.Info().Msg("application stopped")
}

func run(logger zerolog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	httpConfig := settings.Get(config.HTTPKey).(config.HTTPConfig)
	appConfig := settings.Get(config.AppKey).(config.AppConfig)
	postgresConfig := settings.Get(config.PostgresKey).(config.PostgresConfig)

	pool, err := database.NewPool(ctx, postgresConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info().Msg("connected-to-PostgreSQL")

	linkRepository := postgres.NewLinkRepository(pool)
	linkService := service.NewLinkService(linkRepository, appConfig.BaseURL)
	linkHandler := handler.NewLinkHandler(linkService, logger)
	router := handler.NewRouter(linkHandler)

	return server.Run(ctx, httpConfig, router, logger)
}
