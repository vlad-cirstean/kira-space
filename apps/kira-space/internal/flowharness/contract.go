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

// Contract compares got with key in apps/kira-space/tests/contract/<scenario>.json (or writes it
// under KIRA_CONTRACT=write), with this app's temp directories replaced by placeholders. The UI
// specs read the same file as the mock answer; see flowtest.Contract.
func (a *App) Contract(t testing.TB, scenario, key string, got any, opts ...ContractOption) {
	t.Helper()
	a.contract(t, "", scenario, key, got, opts...)
}

// ContractEvent stores ev's payload under key "event:<channel>".
func (a *App) ContractEvent(t testing.TB, scenario string, ev Event, opts ...ContractOption) {
	t.Helper()
	a.Contract(t, scenario, "event:"+ev.Channel, ev.Data, opts...)
}

// StudioContract is Contract for a Studio flow: the file lives in tests/contract/studio and the
// Studio home reads as <kira>.
func (a *App) StudioContract(t testing.TB, scenario, key string, got any, opts ...ContractOption) {
	t.Helper()
	a.contract(t, "studio", scenario, key, got, opts...)
}

// StudioContractEvent is ContractEvent for a Studio flow.
func (a *App) StudioContractEvent(t testing.TB, scenario string, ev Event, opts ...ContractOption) {
	t.Helper()
	a.StudioContract(t, scenario, "event:"+ev.Channel, ev.Data, opts...)
}

// contract compares against the contract tree sub ("" or "studio"); each tree keeps its own
// placeholder set so existing files stay byte-identical.
func (a *App) contract(t testing.TB, sub, scenario, key string, got any, opts ...ContractOption) {
	t.Helper()
	repl := []ContractOption{
		Replace(a.Work, "<work>"), Replace(a.BinDir, "<bin>"), Replace(a.FakeDir, "<fake>"),
		Replace(a.SpaceHome, "<space>"), Replace(a.MemoryHome, "<memory>"), Replace(a.Home, "<home>"),
		Replace(a.Root, "<tmp>"),
	}
	if sub == "studio" {
		repl = []ContractOption{
			Replace(a.BinDir, "<bin>"), Replace(a.FakeDir, "<fake>"), Replace(a.KiraHome, "<kira>"), Replace(a.SpaceHome, "<kira>"),
			Replace(a.Home, "<home>"), Replace(a.Root, "<tmp>"),
		}
	}
	flowtest.Contract(t, contractDir(sub), scenario, key, got, append(repl, opts...)...)
}

func contractDir(sub string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "tests", "contract", sub)
}
