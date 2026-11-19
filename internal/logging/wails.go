package logging

import (
	"context"
	"log/slog"
	"strings"
)

// bindingCallPrefix starts the message of Wails' per-call debug records ("Binding call complete:").
const bindingCallPrefix = "Binding call"

// bindingSecretAttrs are the attrs of those records that carry the call's arguments and result as JSON:
// an approval answer holding a typed value, a reveal holding a plaintext.
var bindingSecretAttrs = map[string]bool{"args": true, "result": true}

// WailsLogger wraps base for application.Options.Logger. Wails logs every bound call at Debug with its
// arguments and result; this drops those two attrs, so a dev build never prints a value a user typed in an
// approval or a reveal returned. Every other record passes through unchanged.
func WailsLogger(base *slog.Logger) *slog.Logger {
	return slog.New(&bindingRedactor{next: base.Handler()})
}

type bindingRedactor struct{ next slog.Handler }

func (h *bindingRedactor) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *bindingRedactor) Handle(ctx context.Context, r slog.Record) error {
	if !strings.HasPrefix(r.Message, bindingCallPrefix) {
		return h.next.Handle(ctx, r)
	}
	clean := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if !bindingSecretAttrs[a.Key] {
			clean.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h *bindingRedactor) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &bindingRedactor{next: h.next.WithAttrs(attrs)}
}

func (h *bindingRedactor) WithGroup(name string) slog.Handler {
	return &bindingRedactor{next: h.next.WithGroup(name)}
}
