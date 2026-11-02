package opencode

import (
	"encoding/json"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// event is one `run --format json` line, reduced to what Kira uses.
type event struct {
	// SessionID is the conversation id the line carried, "" when none.
	SessionID string
	Lines     []harness.Line
	// Err is the message of an error event, "" for any other line.
	Err  string
	Cost *float64
	// Text is the line's assistant text, for the closing result.
	Text string
}

// parseEvent reads one stdout line (U3, partly verified: every line has type, timestamp and a
// top-level sessionID; the error event is {type:"error",error:{name,data:{message}}}). The text,
// tool and cost shapes are tolerant guesses: text at part.text or text, tool at part.tool, cost at
// cost or part.cost. ok is false for a line that is not a JSON object with a type.
func parseEvent(raw string) (ev event, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return event{}, false
	}
	var m struct {
		Type      string          `json:"type"`
		SessionID string          `json:"sessionID"`
		Text      string          `json:"text"`
		Cost      *float64        `json:"cost"`
		Error     json.RawMessage `json:"error"`
		Part      struct {
			SessionID string   `json:"sessionID"`
			Text      string   `json:"text"`
			Tool      string   `json:"tool"`
			Cost      *float64 `json:"cost"`
		} `json:"part"`
	}
	if json.Unmarshal([]byte(raw), &m) != nil || m.Type == "" {
		return event{}, false
	}
	ev.SessionID = m.SessionID
	if ev.SessionID == "" {
		ev.SessionID = m.Part.SessionID
	}
	ev.Cost = m.Cost
	if ev.Cost == nil {
		ev.Cost = m.Part.Cost
	}
	if msg, isErr := errorText(m.Type, m.Error); isErr {
		ev.Err = msg
		ev.Lines = []harness.Line{{Stream: harness.StreamStderr, Text: msg}}
		return ev, true
	}
	text := m.Part.Text
	if text == "" {
		text = m.Text
	}
	switch {
	case m.Type == "text" && text != "":
		ev.Text = text
		for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			ev.Lines = append(ev.Lines, harness.Line{Stream: harness.StreamStdout, Text: l})
		}
	case strings.Contains(m.Type, "tool") && m.Part.Tool != "":
		ev.Lines = []harness.Line{{Stream: harness.StreamEvent, Text: "tool: " + m.Part.Tool}}
	}
	return ev, true
}

// errorText is the message of an error event: error.data.message, else error.name, else a plain
// string error. isErr is true for type "error" or any line with an error field.
func errorText(typ string, raw json.RawMessage) (msg string, isErr bool) {
	if typ != "error" && len(raw) == 0 {
		return "", false
	}
	var obj struct {
		Name string `json:"name"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		switch {
		case obj.Data.Message != "":
			return obj.Data.Message, true
		case obj.Message != "":
			return obj.Message, true
		case obj.Name != "":
			return obj.Name, true
		}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s, true
	}
	return "agent reported an error", typ == "error" || len(raw) > 0
}

// ResumeLost reports whether an error line says the resumed conversation does not exist (U5,
// UNVERIFIED).
func (*Provider) ResumeLost(line string) bool {
	l := strings.ToLower(line)
	for _, m := range resumeLostMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
