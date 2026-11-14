package gitflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

// J-BC-009: the values a user copies from a commit are git's own, in full. Contract git-copy-values:
// tests/ui/repo-commit-detail.spec.ts copies the sha and subject back out of the rendered commit.
func TestCopyValues(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("root", map[string]string{"a.txt": "a\n"})
	const message = "Fix the parser\n\nFirst paragraph of the body.\n\nSecond paragraph.\n\nSigned-off-by: Dev One <dev@example.com>\nCo-authored-by: Dev Two <two@example.com>"
	nfc, nfd := "café.txt", flowharness.NFDName+".txt"
	repo.Commit(message, map[string]string{nfc: "nfc\n", nfd: "nfd\n", "dir/nested file.txt": "n\n"})
	id := r.open(repo.Dir).RepoID

	full := strings.TrimSpace(repo.Git("rev-parse", "HEAD"))
	if len(full) != 40 {
		t.Fatalf("rev-parse HEAD = %q", full)
	}
	detail := call[porcelain.CommitDetail](t, r.gs, "commit.detail", gitrpc.CommitDetailParams{RepoID: id, SHA: full})
	if detail.SHA != full {
		t.Fatalf("detail sha = %q (%d chars), want the full %s", detail.SHA, len(detail.SHA), full)
	}
	if want := strings.TrimSpace(repo.Git("log", "-1", "--format=%s")); detail.Subject != want {
		t.Fatalf("subject = %q, want %q", detail.Subject, want)
	}
	// Pin: subject, body and the trailer block, each after a blank line, rebuild git's %B without its
	// final newline. The body field holds no trailers.
	rebuilt := detail.Subject
	if detail.Body != "" {
		rebuilt += "\n\n" + detail.Body
	}
	var trailers []string
	for _, tr := range detail.Trailers {
		trailers = append(trailers, tr.Token+": "+tr.Value)
	}
	if len(trailers) > 0 {
		rebuilt += "\n\n" + strings.Join(trailers, "\n")
	}
	if want := strings.TrimRight(repo.Git("log", "-1", "--format=%B"), "\n"); rebuilt != want {
		t.Fatalf("subject + body = %q, git %%B = %q", rebuilt, want)
	}
	if len(detail.Trailers) != 2 || detail.Trailers[0].Token != "Signed-off-by" {
		t.Fatalf("trailers = %+v", detail.Trailers)
	}

	// Paths come back exactly as git stores them: NFC and NFD stay distinct, a space survives.
	want := map[string]bool{}
	for _, p := range strings.Split(strings.TrimRight(repo.Git("show", "-z", "--name-only", "--format=", full), "\x00"), "\x00") {
		want[p] = true
	}
	got := map[string]bool{}
	for _, f := range detail.Files {
		got[f.Path] = true
	}
	for p := range want {
		if !got[p] {
			t.Errorf("commit.detail lacks path %q (git: %v, got: %v)", p, want, got)
		}
	}
	if !got[nfc] || !got[nfd] || nfc == nfd {
		t.Fatalf("NFC and NFD paths = %v", got)
	}
	r.app.Contract(t, "git-copy-values", "git:commit.detail#copy", detail, flowharness.Omit("files", "decoration", "committer", "author", "signature"))
}
