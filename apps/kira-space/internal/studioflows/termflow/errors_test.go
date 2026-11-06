package termflow

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Mirrors tests/ui/consistency-docker-automations.spec.ts: the run dialog shows this text.
func TestPreviewRefusesTaskInStudio(t *testing.T) {
	app := studioflowharness.New(t)
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "plain", Kind: scripts.KindScript, Command: "echo hi", Color: "blue", DirMode: scripts.DirModeKira,
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.W.ScriptRuns.Preview(scriptruns.RunArgs{ScriptID: rec.ID, TaskID: "task-1"})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *ipcerr.Error", err)
	}
	if ie.Code != "E_BAD_REQUEST" || ie.Message != "Tasks exist only in Kira Space." {
		t.Fatalf("err = %q %q, want E_BAD_REQUEST and the catalog text", ie.Code, ie.Message)
	}
}
