// layering_test.go guards the same Go-side module-boundary rule Kira Studio's own copy does
// (apps/kira-space/internal/layering_test.go): internal/bridge is the IPC transport layer, and
// no domain package below it may import anything under internal/bridge.
package internal_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/layeringtest"
)

const modulePrefix = "github.com/kirathecat/kira-studio/apps/kira-space/"

// packagesExemptFromBridgeCheck are the packages that sit at or above internal/bridge in the
// intended layering. internal/appshell replaces internal/shell here — the generic
// half of the app's own composition root moved to repo-root internal/shell, outside this go list
// scope; appshell is what's left importing bridge. internal/appwire is the shared composition
// root and internal/flowharness the flow-test harness over it, the same role as Kira Studio's
// internal/ipcfixture; the flows/* packages hold only tests of the bound services.
var packagesExemptFromBridgeCheck = map[string]bool{
	"internal":                       true,
	"internal/bridge":                true,
	"internal/appshell":              true,
	"internal/appwire":               true,
	"internal/flowharness":           true,
	"internal/ipcfixture":            true,
	"internal/studioflowharness":     true,
	"internal/flowharness/fakeagent": true,
	"internal/flows/gitflow":         true,
	"internal/flows/repoflow":        true,
	"internal/flows/adeflow":         true,
	"internal/flows/memoryflow":      true,
	"internal/flows/termflow":        true,
	"internal/flows/appflow":         true,
	"internal/flows/mobileflow":      true,
}

// adewire is a pure wire-type leaf the v2 ADE engine returns directly, so domain packages
// may import it; every other internal/bridge dependency stays banned.
//
// TestDomainPackagesDoNotImportBridge is Kira Studio's own test (see its own doc comment for the
// full rationale), enumerating every internal/* package under this app straight from `go list`.
// The runner itself is shared (internal/layeringtest.Run) — only modulePrefix and the
// exemption set differ per app.
func TestDomainPackagesDoNotImportBridge(t *testing.T) {
	layeringtest.RunAllowing(t, modulePrefix, packagesExemptFromBridgeCheck, []string{"/internal/bridge/adewire"})
}
