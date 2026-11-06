// layering_test.go guards the Go-side module-boundary rule: internal/bridge is the IPC transport layer, and
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
// root and internal/flowharness the flow-test harness over it, the same role as internal/ipcfixture;
// the flows/* packages hold only tests of the bound services.
var packagesExemptFromBridgeCheck = map[string]bool{
	"internal":                       true,
	"internal/bridge":                true,
	"internal/appshell":              true,
	"internal/appwire":               true,
	"internal/flowharness":           true,
	"internal/ipcfixture":            true,
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
// TestDomainPackagesDoNotImportBridge enumerates every internal/* package straight from `go list`.
// The runner is shared (internal/layeringtest.Run).
func TestDomainPackagesDoNotImportBridge(t *testing.T) {
	layeringtest.RunAllowing(t, modulePrefix, packagesExemptFromBridgeCheck, []string{"/internal/bridge/adewire"})
}
