package tlog

import (
	"context"
	"k0pern1cus/pkg/util"
	"log/slog"
)

var _ slog.Handler = (*contextHandler)(nil)

type contextHandler struct {
	handler slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{
		handler: h.handler.WithAttrs(attrs),
	}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{
		handler: h.handler.WithGroup(name),
	}
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if username, ok := ctx.Value(util.UsernameContextKey).(string); ok {
		r.AddAttrs(slog.String("username", username))
	}

	if ip, ok := ctx.Value(util.IpContextKey).(string); ok {
		r.AddAttrs(slog.String("ip", ip))
	}

	return h.handler.Handle(ctx, r) //nolint: wrapcheck
}
