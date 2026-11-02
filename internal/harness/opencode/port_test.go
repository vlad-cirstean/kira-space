package opencode

import (
	"encoding/json"
	"testing"
)

// The last matching rule wins, so merge order decides whether a Kira allow can widen a user deny.
func TestMergePermission(t *testing.T) {
	for _, c := range []struct{ name, user, add, want string }{
		{"broad allow replaces the user star and stays before a deny", `{"*":"ask","git push *":"deny"}`, `"allow"`, `{"*":"allow","git push *":"deny"}`},
		{"narrow allow lands before the first deny", `{"*":"ask","git push *":"deny"}`, `{"git status *":"allow"}`, `{"*":"ask","git status *":"allow","git push *":"deny"}`},
		{"same pattern as a user deny is dropped", `{"git push *":"deny"}`, `{"git push *":"allow"}`, `{"git push *":"deny"}`},
		{"no user deny appends", `{"*":"ask"}`, `{"git *":"allow"}`, `{"*":"ask","git *":"allow"}`},
		{"kira deny goes last", `{"git push *":"deny","*":"ask"}`, `{"rm *":"deny"}`, `{"git push *":"deny","*":"ask","rm *":"deny"}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := MergePermission(json.RawMessage(c.user), json.RawMessage(c.add))
			if err != nil || string(got) != c.want {
				t.Fatalf("MergePermission = %s, %v, want %s", got, err, c.want)
			}
		})
	}
}
