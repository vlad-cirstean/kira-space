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

// Validation proof sits in the Window section as a Go-side action, so it works with no window focused.
func TestBuildTemplateValidationProofItem(t *testing.T) {
	for _, s := range BuildTemplate("Kira Space", false) {
		for i, it := range s.Items {
			if it.Kind != shell.ItemAction {
				continue
			}
			if it.Action != ActionOpenProofs || it.Label != "Validation proof" || it.Accelerator != "" {
				t.Fatalf("action item = %+v", it)
			}
			if prev := s.Items[i-1]; prev.Kind != shell.ItemNewWindow {
				t.Fatalf("item before = %+v, want New Window", prev)
			}
			return
		}
	}
	t.Fatal("no action menu item")
}
