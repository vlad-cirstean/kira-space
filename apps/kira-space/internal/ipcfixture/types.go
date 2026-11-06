package ipcfixture

import "encoding/json"

// ControlSnapshot mirrors tests/ipc/support/types.ts's ControlSnapshot. Args/Response are raw JSON
// rather than typed fields: each call site already has a concrete Go value to marshal (the bridge
// service's own request/response struct), and json.RawMessage preserves that struct's field order
// verbatim into the written fixture (write.go's second trap) without this package needing its own
// copy of every bridge type's shape. omitempty on both matches encoding/json's own behaviour for a
// TypeScript `undefined`-valued key (JSON.stringify drops it; the Go writer must too).
type ControlSnapshot struct {
	Channel  string          `json:"channel"`
	Args     json.RawMessage `json:"args,omitempty"`
	Response json.RawMessage `json:"response,omitempty"`
}

// PortSnapshot mirrors tests/ipc/support/types.ts's PortSnapshot.
type PortSnapshot struct {
	Op       string          `json:"op"`
	Payload  json.RawMessage `json:"payload"`
	Response json.RawMessage `json:"response,omitempty"`
	DelayMs  *int            `json:"delayMs,omitempty"`
}

// rawJSON marshals v with HTML-escaping disabled (write.go's first trap — the fixtures contain SQL
// and JSON text) and panics on a marshal failure, which every caller here can only hit by passing
// a value encoding/json genuinely cannot encode — a programmer error, not a runtime condition a
// fixture-generating test should recover from.
func rawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	var buf []byte
	buf = mustMarshalNoEscape(v)
	return json.RawMessage(buf)
}
