package appshell

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// Each global chord is a menu accelerator on an item that emits its channel, so the renderer needs
// no window keydown. New Connection and New Request keep their items but hold no chord: ⌘N is the
// contextual New.
func TestBuildTemplateGlobalChordItems(t *testing.T) {
	want := map[string]string{
		bridge.ChannelReviewCode:      "ade.reviewCode",
		bridge.ChannelNew:             "app.new",
		bridge.ChannelNewAlternate:    "app.newAlternate",
		bridge.ChannelImport:          "app.import",
		bridge.ChannelApiEnvironments: "api.environments",
		bridge.ChannelCheckForUpdates: "app.checkForUpdates",
	}
	noChord := []string{bridge.ChannelNewConnection, bridge.ChannelNewRequest}

	found := map[string]shell.Item{}
	for _, s := range BuildTemplate("Kira Space", false) {
		for _, it := range s.Items {
			if it.Kind == shell.ItemEmit {
				found[it.Channel] = it
			}
		}
	}
	for channel, id := range want {
		it, ok := found[channel]
		if !ok {
			t.Errorf("no menu item emits %s", channel)
			continue
		}
		if wantAccel := shell.Shortcuts[id].Accelerator(); it.Accelerator != wantAccel {
			t.Errorf("%s accelerator = %q, want %q", channel, it.Accelerator, wantAccel)
		}
	}
	for _, channel := range noChord {
		it, ok := found[channel]
		if !ok {
			t.Errorf("no menu item emits %s", channel)
		} else if it.Accelerator != "" {
			t.Errorf("%s accelerator = %q, want none", channel, it.Accelerator)
		}
	}
}
