package main

import (
	"context"
	"log/slog"
	"os"

	"codeberg.org/cuducos/minha-receita/cmd"
)

type handler struct {
	stdout slog.Handler
	stderr slog.Handler
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return true }

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{h.stdout.WithAttrs(attrs), h.stderr.WithAttrs(attrs)}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{h.stdout.WithGroup(name), h.stderr.WithGroup(name)}
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		return h.stderr.Handle(ctx, r)
	}
	return h.stdout.Handle(ctx, r)
}

func main() {
	l := slog.LevelInfo
	if os.Getenv("DEBUG") != "" {
		l = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: l}
	h := &handler{slog.NewTextHandler(os.Stdout, opts), slog.NewTextHandler(os.Stderr, opts)}
	slog.SetDefault(slog.New(h))
	if err := cmd.CLI().Execute(); err != nil {
		slog.Error("Exiting minha-receita", "error", err)
		os.Exit(1)
	}
}
