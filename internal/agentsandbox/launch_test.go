package agentsandbox

import (
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/internal/harness"
)

type denyProbe struct{ readOnly bool }

func (*denyProbe) SandboxNeeds(string, string) harness.SandboxNeeds { return harness.SandboxNeeds{} }

func (*denyProbe) APIHosts(func(string) string) (exact, suffixes []string) { return nil, nil }

func (d *denyProbe) SandboxDenies(readOnly bool, _ []string, _ bool) []string {
	d.readOnly = readOnly
	return nil
}

// A read-access run that has an allowed folder (the task folder) must not get the blanket tool deny,
// which would beat the --add-dir grant; a read run with none keeps it.
func TestFlagsReadAccessTaskFolder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		allow []string
		want  bool
	}{{"no allowed folder", nil, true}, {"task folder allowed", []string{"/tasks/t1"}, false}} {
		var d denyProbe
		addDirs, _ := Flags(Policy{Access: AccessRead, Allow: tc.allow, Read: []string{"/wt"}, Scratch: "/s"}, &d)
		if d.readOnly != tc.want {
			t.Errorf("%s: readOnly = %v, want %v", tc.name, d.readOnly, tc.want)
		}
		if tc.allow != nil && !slices.Contains(addDirs, "/tasks/t1") {
			t.Errorf("%s: add-dir %v lacks the task folder", tc.name, addDirs)
		}
	}
}
