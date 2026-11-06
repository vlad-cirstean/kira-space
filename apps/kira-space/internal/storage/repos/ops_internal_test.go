package repos

import "testing"

// SetOpLogByteBudgetForTest overrides the table-wide byte budget for the duration of one test,
// restoring the real value on cleanup — a verbatim port of response_history_internal_test.go's
// SetHistoryByteBudgetForTest, for the same reason: the only production reader of opLogByteBudget
// is Prune's own sweep, so this is the one way an external (repos_test) test can exercise that
// sweep for real without reproducing 32 MiB of real rows.
func SetOpLogByteBudgetForTest(t *testing.T, n int) {
	t.Helper()
	orig := opLogByteBudget
	opLogByteBudget = n
	t.Cleanup(func() { opLogByteBudget = orig })
}
