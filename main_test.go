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
		wantOutput bool
	}{
		{"debug filtrado quando nível é Info", slog.LevelInfo, slog.LevelDebug, false},
		{"info passa quando nível é Info", slog.LevelInfo, slog.LevelInfo, true},
		{"debug passa quando nível é Debug", slog.LevelDebug, slog.LevelDebug, true},
		{"error passa quando nível é Info", slog.LevelInfo, slog.LevelError, true},
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

			logger.Log(t.Context(), tc.logged, "mensagem de teste")

			gotOutput := buf.Len() > 0
			if gotOutput != tc.wantOutput {
				t.Errorf(
					"nível configurado %s, log %s: saída=%v, esperado=%v\nbuffer: %q",
					tc.configured, tc.logged, gotOutput, tc.wantOutput, buf.String(),
				)
			}
		})
	}
}
