package kibana

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// errorBody is Kibana's error envelope. Attributes.Error carries the Elasticsearch error on a
// search route.
type errorBody struct {
	StatusCode int    `json:"statusCode"`
	Error      string `json:"error"`
	Message    string `json:"message"`
	Attributes struct {
		Error *esError `json:"error"`
	} `json:"attributes"`
}

type esError struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	RootCause []esError `json:"root_cause"`
	CausedBy  *esError  `json:"caused_by"`
}

// message prefers the Elasticsearch root cause: Kibana's own message concatenates the whole chain.
func (b errorBody) message(status int, raw []byte) string {
	if e := b.Attributes.Error; e != nil {
		if len(e.RootCause) > 0 && e.RootCause[0].Reason != "" {
			return e.RootCause[0].Reason
		}
		if e.CausedBy != nil && e.CausedBy.Reason != "" {
			return e.CausedBy.Reason
		}
		if e.Reason != "" {
			return e.Reason
		}
	}
	if b.Message != "" {
		return b.Message
	}
	if text := strings.TrimSpace(string(raw)); text != "" {
		return text
	}
	return "Kibana answered " + strconv.Itoa(status)
}

// mapStatus turns a non-2xx Kibana response into an adapter error.
func mapStatus(status int, raw []byte) error {
	var body errorBody
	_ = json.Unmarshal(raw, &body)
	msg := body.message(status, raw)
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return adapters.New(adapters.CodeAuth, msg, nil)
	case status == http.StatusNotFound:
		return adapters.New(adapters.CodeNotFound, msg, nil)
	case status == http.StatusRequestTimeout, status == http.StatusGatewayTimeout:
		return adapters.New(adapters.CodeTimeout, msg, nil)
	case status >= 500:
		return adapters.New(adapters.CodeEngineDown, msg, nil)
	default:
		return adapters.New(adapters.CodeQuery, msg, nil)
	}
}

// mapTransport turns a request error into an adapter error. The caller's ctx decides cancelled
// versus timed out; anything else the transport reports is a failed connect.
func mapTransport(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return adapters.T(adapters.CodeCancelled, "backend.studio.adapter.cancelled.body", nil, err)
	case errors.Is(err, context.DeadlineExceeded):
		return adapters.New(adapters.CodeTimeout, err.Error(), err)
	}
	if ne := adapters.ClassifyNetError(err); ne.NetTimeout {
		return adapters.New(adapters.CodeTimeout, err.Error(), err)
	}
	return adapters.New(adapters.CodeConnect, err.Error(), err)
}
