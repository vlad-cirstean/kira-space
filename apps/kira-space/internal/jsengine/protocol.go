package jsengine

import "encoding/json"

// Wire messages, NDJSON on the worker's stdin and stdout.

// workerRequest is the single parent-to-worker request after the hello.
type workerRequest struct {
	Source     string        `json:"source"`
	Bindings   []wireBinding `json:"bindings"`
	Limits     Limits        `json:"limits"`
	TimeoutNs  int64         `json:"timeoutNs"` // remaining wall clock at send time
	NowMs      int64         `json:"nowMs"`
	Seed       uint64        `json:"seed"`
	Marker     string        `json:"marker"` // hex; the worker prefixes NUL
	InputBytes int           `json:"inputBytes"`
}

type wireBinding struct {
	Name     string          `json:"name"`
	Value    json.RawMessage `json:"value,omitempty"`
	Funcs    []string        `json:"funcs,omitempty"`
	ReadBack bool            `json:"readBack,omitempty"`
}

// workerMsg is any worker-to-parent line; exactly one field is set.
type workerMsg struct {
	Ready  bool       `json:"ready,omitempty"`
	Call   *callMsg   `json:"call,omitempty"`
	Result *resultMsg `json:"result,omitempty"`
	Error  *Error     `json:"error,omitempty"`
}

type callMsg struct {
	ID      int             `json:"id"`
	Binding string          `json:"binding"`
	Fn      string          `json:"fn"`
	Args    json.RawMessage `json:"args"`
}

type resultMsg struct {
	JSON      json.RawMessage     `json:"json"`
	Type      string              `json:"type"`
	ElapsedNs int64               `json:"elapsedNs"`
	ReadBack  map[string]ReadBack `json:"readBack,omitempty"`
}

// parentMsg is a parent-to-worker reply to a call.
type parentMsg struct {
	Reply *replyMsg `json:"reply"`
}

type replyMsg struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}
