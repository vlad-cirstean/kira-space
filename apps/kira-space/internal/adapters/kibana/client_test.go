package kibana

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

func TestRequestRejectsOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`"` + strings.Repeat("a", maxResponseBytes) + `"`))
	}))
	defer srv.Close()
	h := &Handle{Client: srv.Client(), Base: srv.URL}
	var out string
	err := h.getJSON(context.Background(), "/x", &out)
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Fatalf("err = %v, want CodeQuery", err)
	}
}
