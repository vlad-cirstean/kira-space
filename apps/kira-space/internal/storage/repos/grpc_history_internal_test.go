package repos

import "testing"

// SetGrpcHistoryByteBudgetForTest overrides the global byte budget for the duration of one
// test, restoring the real value on cleanup — mirrors response_history_internal_test.go's own
// SetHistoryByteBudgetForTest, the one way an external (repos_test) test can exercise Record's
// real sweep without reproducing 32 MiB of rows.
func SetGrpcHistoryByteBudgetForTest(t *testing.T, n int) {
	t.Helper()
	orig := grpcHistoryByteBudget
	grpcHistoryByteBudget = n
	t.Cleanup(func() { grpcHistoryByteBudget = orig })
}
