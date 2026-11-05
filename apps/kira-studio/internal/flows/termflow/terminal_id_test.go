package termflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

func TestTerminalIDBounds(t *testing.T) {
	app := flowharness.New(t)
	if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	open := func(id string) error {
		_, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: "w1", Cwd: app.Home, Cols: 80, Rows: 24})
		return err
	}
	for _, bad := range []string{strings.Repeat("a", 65), "line\nbreak", "has space"} {
		var ie *ipcerr.Error
		if err := open(bad); !errors.As(err, &ie) || ie.Code != "E_CONTRACT" {
			t.Fatalf("Open(%q) = %v, want E_CONTRACT", bad, err)
		}
	}
	if err := open(ids.NewID()); err != nil {
		t.Fatalf("Open with a v7 id: %v", err)
	}
}
