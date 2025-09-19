package sentry

import (
	"k0pern1cus/pkg/build"
	"k0pern1cus/pkg/config"
	"log/slog"

	"github.com/getsentry/sentry-go"
)

func Init(cfg *config.Config) error {
	if cfg.Sentry.DSN == "" {
		slog.Info("Sentry DSN not configured, skipping initialization")
		return nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.Sentry.DSN,
		Environment:      cfg.Sentry.Environment,
		TracesSampleRate: cfg.Sentry.TracesSampleRate,
		Release:          build.Tag,
		AttachStacktrace: true,
	})
	if err != nil {
		return err
	}

	slog.Info("Sentry initialized successfully",
		slog.String("environment", cfg.Sentry.Environment),
		slog.String("release", build.Tag),
	)

	return nil
}
