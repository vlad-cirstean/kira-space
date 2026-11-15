package appshell

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// ade.reviewCode is a global chord: the Review Code item carries its accelerator and channel so the
// renderer needs no window keydown.
func TestBuildTemplateReviewCodeAccelerator(t *testing.T) {
	for _, s := range BuildTemplate("Kira Space", false) {
		for _, it := range s.Items {
			if it.Kind == shell.ItemEmit && it.Channel == bridge.ChannelReviewCode {
				if it.Accelerator != "CmdOrCtrl+Shift+R" {
					t.Fatalf("accelerator = %q, want CmdOrCtrl+Shift+R", it.Accelerator)
				}
				return
			}
		}
	}
	t.Fatal("no Review Code menu item")
}
