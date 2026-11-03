package apifiles

import (
	"errors"
	"strings"
	"testing"
)

const editSrc = `# top comment
name: Shop
items:
  # first folder
  - id: orders
    name: Orders
    items:
      - id: list
        name: List
        http:
          method: GET
          url: https://x/orders
  - id: ping
    name: Ping
    http:
      method: GET
      url: https://x/ping
`

func TestReplaceRequestKeepsComments(t *testing.T) {
	out, err := replaceRequest([]byte(editSrc), "list", "List all", &HTTP{Method: "POST", URL: "https://x/new"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{"# top comment", "# first folder", "name: List all", "https://x/new", "id: ping"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "https://x/orders") {
		t.Errorf("old node kept:\n%s", got)
	}
	if strings.Index(got, "id: orders") > strings.Index(got, "id: ping") {
		t.Errorf("order changed:\n%s", got)
	}
}

func TestReplaceRequestErrors(t *testing.T) {
	if _, err := replaceRequest([]byte(editSrc), "nope", "x", &HTTP{Method: "GET"}, nil); !errors.Is(err, ErrItemGone) {
		t.Errorf("gone: %v", err)
	}
	if _, err := replaceRequest([]byte(editSrc), "ping", "x", nil, &GRPC{}); !errors.Is(err, ErrProtocol) {
		t.Errorf("protocol: %v", err)
	}
}

func TestAppendItem(t *testing.T) {
	out, err := appendItem([]byte(editSrc), "orders", Item{ID: "get", Name: "Get", HTTP: &HTTP{Method: "GET", URL: "https://x/o/1"}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "# first folder") || strings.Index(got, "id: get") > strings.Index(got, "id: ping") {
		t.Errorf("not nested under orders:\n%s", got)
	}
	if _, err := appendItem([]byte(editSrc), "ping", Item{ID: "x", Name: "x"}); !errors.Is(err, ErrNotFolder) {
		t.Errorf("not folder: %v", err)
	}
	if _, err := appendItem([]byte(editSrc), "zzz", Item{ID: "x", Name: "x"}); !errors.Is(err, ErrItemGone) {
		t.Errorf("gone: %v", err)
	}
}
