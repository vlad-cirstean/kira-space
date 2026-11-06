package repos

import "testing"

// SetHistoryByteBudgetForTest overrides the global byte budget for the duration of one test,
// restoring the real value on cleanup. The only production reader of historyByteBudget is
// Record's own sweep, so this is the one way an external (repos_test) test can exercise that
// sweep for real without reproducing 128 MiB of rows — response_history_test.go's cross-scope
// eviction case.
func SetHistoryByteBudgetForTest(t *testing.T, n int) {
	t.Helper()
	orig := historyByteBudget
	historyByteBudget = n
	t.Cleanup(func() { historyByteBudget = orig })
}
