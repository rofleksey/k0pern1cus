package tlog

import (
	"k0pern1cus/pkg/build"
	"k0pern1cus/pkg/config"
	"log/slog"
	"os"

	slogmulti "github.com/samber/slog-multi"
	slogtelegram "github.com/samber/slog-telegram/v2"
)

func Init(cfg *config.Config) error {
	logHandlers := []slog.Handler{slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	})}

	if cfg.Log.Telegram.Token != "" && cfg.Log.Telegram.ChatID != "" {
		logHandlers = append(logHandlers, slogtelegram.Option{
			Level:     slog.LevelError,
			Token:     cfg.Log.Telegram.Token,
			Username:  cfg.Log.Telegram.ChatID,
			AddSource: true,
		}.NewTelegramHandler())
	}

	multiHandler := slogmulti.Fanout(logHandlers...)
	ctxHandler := &contextHandler{multiHandler}

	logger := slog.New(ctxHandler).With(
		slog.String("app", "api"),
		slog.String("app_tag", build.Tag),
	)
	slog.SetDefault(logger)

	return nil
}
