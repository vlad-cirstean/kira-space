package ade

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Golden parity with frontend/src/ade/v2/board/taskOrder.ts: cases.json is written by
//
//	UPDATE_TASK_ORDER=1 bun test apps/kira-space/tests/unit/ade-v2-task-order-parity.spec.ts
//
// from the real TS function. After a TS order change, regenerate it and port the change to
// taskorder.go until this test passes.
func TestTaskListOrder(t *testing.T) {
	raw, err := os.ReadFile("testdata/taskorder/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string            `json:"name"`
		Today    string            `json:"today"`
		Settings model.AdeSettings `json:"settings"`
		Board    adewire.Board     `json:"board"`
		Order    []string          `json:"order"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 220 {
		t.Fatalf("cases.json holds %d cases, want at least 220", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := TaskListOrder(c.Board, c.Settings, c.Today)
			if !slices.Equal(got, c.Order) {
				t.Fatalf("order = %v, want %v", got, c.Order)
			}
		})
	}
}
