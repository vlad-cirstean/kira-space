package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestWailsLoggerDropsBindingArgsAndResult(t *testing.T) {
	var buf bytes.Buffer
	log := WailsLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.Debug("Binding call complete:", "id", "7", "method", "ApprovalsService.Answer", "args", `[{"inputs":{"token":"PLAINTEXT-9c41"}}]`, "result", `{"value":"PLAINTEXT-9c41"}`)
	log.With("scope", "x").Debug("Binding call started:", "args", "PLAINTEXT-9c41")
	log.Debug("Binding call cancelled:", "id", "8")
	out := buf.String()
	if strings.Contains(out, "PLAINTEXT-9c41") || strings.Contains(out, "args=") || strings.Contains(out, "result=") {
		t.Fatalf("binding record leaked: %s", out)
	}
	for _, keep := range []string{"method=ApprovalsService.Answer", "id=7", "scope=x", "id=8"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("lost %q: %s", keep, out)
		}
	}
}

func TestWailsLoggerKeepsOtherRecords(t *testing.T) {
	var buf bytes.Buffer
	log := WailsLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	log.Info("window created", "args", "kept", "result", "kept")
	if got := buf.String(); !strings.Contains(got, "args=kept") || !strings.Contains(got, "result=kept") {
		t.Fatalf("other record altered: %s", got)
	}
}
