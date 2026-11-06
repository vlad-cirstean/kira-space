package flowharness

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kirathecat/kira-studio/internal/flowtest"
)

// ContractOption tunes Contract; see flowtest.Contract.
type ContractOption = flowtest.ContractOption

// Mask, Omit and Replace re-export the flowtest options.
var (
	Mask    = flowtest.Mask
	Omit    = flowtest.Omit
	Replace = flowtest.Replace
)

// Contract compares got with key in apps/kira-studio/tests/contract/<scenario>.json (or writes it
// under KIRA_CONTRACT=write), with this app's temp directories replaced by placeholders. The UI
// specs read the same file as the mock answer; see flowtest.Contract.
func (a *App) Contract(t testing.TB, scenario, key string, got any, opts ...ContractOption) {
	t.Helper()
	opts = append([]ContractOption{
		Replace(a.BinDir, "<bin>"), Replace(a.FakeDir, "<fake>"), Replace(a.KiraHome, "<kira>"),
		Replace(a.Home, "<home>"), Replace(filepath.Dir(a.Home), "<tmp>"),
	}, opts...)
	flowtest.Contract(t, contractDir(), scenario, key, got, opts...)
}

// ContractEvent stores ev's payload under key "event:<channel>".
func (a *App) ContractEvent(t testing.TB, scenario string, ev Event, opts ...ContractOption) {
	t.Helper()
	a.Contract(t, scenario, "event:"+ev.Channel, ev.Data, opts...)
}

func contractDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "contract")
}
