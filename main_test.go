package main

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestHandlerEnabled(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		configured slog.Level
		logged     slog.Level
		want       bool
	}{
		{"debug filtered when level is Info", slog.LevelInfo, slog.LevelDebug, false},
		{"info passes when level is Info", slog.LevelInfo, slog.LevelInfo, true},
		{"debug passes when level is Debug", slog.LevelDebug, slog.LevelDebug, true},
		{"error passes when level is Info", slog.LevelInfo, slog.LevelError, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			opts := &slog.HandlerOptions{Level: tc.configured}
			h := &handler{
				slog.NewTextHandler(&buf, opts),
				slog.NewTextHandler(&buf, opts),
			}
			logger := slog.New(h)

			logger.Log(t.Context(), tc.logged, "test message")

			got := buf.Len() > 0
			if got != tc.want {
				t.Errorf(
					"configured level %s, logged %s: output=%v, want=%v\nbuffer: %q",
					tc.configured, tc.logged, got, tc.want, buf.String(),
				)
			}
		})
	}
}
