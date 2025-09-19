package main

import (
	"context"
	"k0pern1cus/app/client/clip_downloader"
	"k0pern1cus/app/client/twitch"
	"k0pern1cus/app/service/clips"
	"k0pern1cus/app/service/streamer"
	"k0pern1cus/pkg/config"
	sentry2 "k0pern1cus/pkg/sentry"
	"k0pern1cus/pkg/tlog"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v2/log"
	"github.com/samber/do"
)

func main() {
	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// stop the stream after a day
	appCtx, cancel1 := context.WithTimeout(appCtx, 24*time.Hour)
	defer cancel1()

	_ = os.RemoveAll("data")
	_ = os.Mkdir("data", os.ModePerm)

	di := do.New()
	do.ProvideValue(di, appCtx)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}
	do.ProvideValue(di, cfg)

	if err = tlog.Init(cfg); err != nil {
		log.Fatalf("logging init failed: %v", err)
	}

	if err = sentry2.Init(cfg); err != nil {
		slog.Error("Sentry initialization failed", slog.Any("error", err))
	}
	defer sentry.Flush(time.Second)
	defer sentry.RecoverWithContext(appCtx)

	slog.ErrorContext(appCtx, "Service restarted")

	do.Provide(di, twitch.NewClient)
	do.Provide(di, clip_downloader.New)
	do.Provide(di, clips.New)
	do.Provide(di, streamer.New)

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Info("Shutting down server...")

		cancel()
	}()

	if err = do.MustInvoke[*clips.Service](di).Init(appCtx); err != nil {
		log.Fatalf("clip service init failed: %v", err)
	}

	if err = do.MustInvoke[*streamer.Service](di).Run(appCtx); err != nil {
		log.Fatalf("streaming failed: %v", err)
	}

	log.Info("Waiting for services to finish...")
	_ = di.Shutdown()
}
