// Package coverage fails when a Wails-bound method has no call in any flow test. Name match, not
// call-graph proof. Exemptions go in exempt.txt as `name: reason`.
package coverage_test

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest"
)

func TestMain(m *testing.M) { os.Exit(flowharness.Main(m)) }

func TestEveryBoundMethodHasAFlow(t *testing.T) {
	app := flowharness.New(t)
	flowtest.CheckCoverage(t, "bound", flowtest.BoundMethods(app.W.Bound()), "..", ".", "exempt.txt")
}
