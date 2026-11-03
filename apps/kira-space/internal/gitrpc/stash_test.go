package gitrpc

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// These tests exercise the real Router.ForConn dispatch (D16/§7.1 item 9) — the same convention
// stack_test.go/worktree_test.go already established.

func stashRpcGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initStashRpcRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stashRpcGit(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stashRpcGit(t, dir, "add", "f.txt")
	stashRpcGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// TestContractVersion_Is58 is P7 item 2's own literal exit-criteria assertion, moved forward again
// (57 -> 58, P464) in the same commit that bumps the constant.
func TestContractVersion_Is58(t *testing.T) {
	if ContractVersion != 58 {
		t.Fatalf("ContractVersion = %d, want 58", ContractVersion)
	}
}

// TestGlobalStashList_EmptyBucketSpawnsExactlyOneForEachRefAndNoLog is §7.1 item 9's own exact exit
// criterion: a Router over a counting fake runner drives app.init/repo.open/refs.list/stash.list,
// then globalStash.list — asserting exactly one `for-each-ref` and zero `log` calls.
func TestGlobalStashList_EmptyBucketSpawnsExactlyOneForEachRefAndNoLog(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := initStashRpcRepo(t)

	runner := newVerbCountingRunner("for-each-ref", "log")
	registry := gitsession.NewRegistry(runner)
	t.Cleanup(registry.Close)
	discovery := gitclient.NewDiscovery(alwaysGitLocator{}, runner, gitclient.NewRealClock())
	router := New(Deps{Discovery: discovery, Runner: runner, Registry: registry, ServerVersion: "test"})
	ctx := context.Background()

	_ = router.handleAppInit(ctx)

	conn := gitsession.NewConn(gitsession.ConnID("stash-rpc-zero-spawn-conn"), "test-client", "test-label", nil)
	t.Cleanup(conn.Close)
	handlers := router.ForConn(conn)

	openParams, _ := json.Marshal(RepoOpenParams{Path: dir})
	openResultAny, err := handlers.Request(ctx, "repo.open", openParams)
	if err != nil {
		t.Fatalf("repo.open: %v", err)
	}
	opened, ok := openResultAny.(RepoOpenResult)
	if !ok || opened.Kind != "ok" || opened.Repo == nil {
		t.Fatalf("repo.open result = %+v, want ok", openResultAny)
	}
	repoID := opened.Repo.RepoID

	refsParams, _ := json.Marshal(RefsListParams{RepoID: repoID})
	if _, err := handlers.Request(ctx, "refs.list", refsParams); err != nil {
		t.Fatalf("refs.list: %v", err)
	}
	stashListParams, _ := json.Marshal(StashListParams{RepoID: repoID})
	if _, err := handlers.Request(ctx, "stash.list", stashListParams); err != nil {
		t.Fatalf("stash.list: %v", err)
	}

	// Reset counters after the setup sequence's own spawns (repo.open/refs.list/stash.list all
	// touch for-each-ref/log themselves) -- only globalStash.list's OWN spawns matter from here.
	for _, v := range []string{"for-each-ref", "log"} {
		runner.counts[v] = new(int32)
	}

	globalParams, _ := json.Marshal(GlobalStashListParams{RepoID: repoID})
	resultAny, err := handlers.Request(ctx, "globalStash.list", globalParams)
	if err != nil {
		t.Fatalf("globalStash.list: %v", err)
	}
	result, ok := resultAny.(StashListResult)
	if !ok {
		t.Fatalf("globalStash.list result = %T, want StashListResult", resultAny)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("got %d entries, want 0 (empty bucket): %+v", len(result.Entries), result.Entries)
	}
	if got := runner.count("for-each-ref"); got != 1 {
		t.Fatalf("for-each-ref spawn count = %d, want exactly 1", got)
	}
	if got := runner.count("log"); got != 0 {
		t.Fatalf("log spawn count = %d, want 0", got)
	}
}

// TestStashHandlers_UnknownScopeIsBadRequestWithNoSpawn proves validStashScope's own gate: an
// unrecognised scope is refused before entryFor is even consulted, let alone any git spawn.
func TestStashHandlers_UnknownScopeIsBadRequestWithNoSpawn(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := initStashRpcRepo(t)

	runner := newVerbCountingRunner("stash", "log", "rev-parse", "for-each-ref", "diff")
	registry := gitsession.NewRegistry(runner)
	t.Cleanup(registry.Close)
	discovery := gitclient.NewDiscovery(alwaysGitLocator{}, runner, gitclient.NewRealClock())
	router := New(Deps{Discovery: discovery, Runner: runner, Registry: registry, ServerVersion: "test"})
	ctx := context.Background()
	_ = router.handleAppInit(ctx)

	conn := gitsession.NewConn(gitsession.ConnID("stash-rpc-bad-scope-conn"), "test-client", "test-label", nil)
	t.Cleanup(conn.Close)
	handlers := router.ForConn(conn)

	openParams, _ := json.Marshal(RepoOpenParams{Path: dir})
	openResultAny, err := handlers.Request(ctx, "repo.open", openParams)
	if err != nil {
		t.Fatalf("repo.open: %v", err)
	}
	repoID := openResultAny.(RepoOpenResult).Repo.RepoID

	// Reset counters after repo.open's own setup spawns -- only the refused request's own spawns
	// (there must be none) matter from here.
	for _, v := range []string{"stash", "log", "rev-parse", "for-each-ref", "diff"} {
		runner.counts[v] = new(int32)
	}

	showParams, _ := json.Marshal(StashShowParams{RepoID: repoID, SHA: "deadbeef", Scope: "nonsense"})
	if _, err := handlers.Request(ctx, "stash.show", showParams); err == nil {
		t.Fatal("stash.show with an unknown scope: want an error")
	}

	for _, v := range []string{"stash", "log", "rev-parse", "for-each-ref", "diff"} {
		if got := runner.count(v); got != 0 {
			t.Fatalf("%s spawn count = %d, want 0 (refused before any spawn)", v, got)
		}
	}
}

// TestPreflightStashPop_GlobalScope_ResolvesFromTheBucket is §7.1 item 10's own exit criterion: a
// scope:'global' request resolves the entry from the bucket and produces a StashPopPreflight whose
// targetSha is the current HEAD (PreflightStashPop's own default when targetSha is omitted).
func TestPreflightStashPop_GlobalScope_ResolvesFromTheBucket(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := initStashRpcRepo(t)
	// Build a global entry directly: dirty the tree, `stash create`, `update-ref` into the bucket.
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello\nglobal change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", dir, "stash", "create", "my global entry").CombinedOutput()
	if err != nil {
		t.Fatalf("git stash create: %v\n%s", err, out)
	}
	sha := string(out)
	for len(sha) > 0 && (sha[len(sha)-1] == '\n' || sha[len(sha)-1] == '\r') {
		sha = sha[:len(sha)-1]
	}
	if sha == "" {
		t.Fatal("git stash create produced no sha")
	}
	stashRpcGit(t, dir, "update-ref", "refs/kira/globalstash/"+sha, sha)
	stashRpcGit(t, dir, "checkout", "-q", "--", "f.txt")
	head := ""
	{
		out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
		if err != nil {
			t.Fatalf("rev-parse HEAD: %v\n%s", err, out)
		}
		head = string(out)
		for len(head) > 0 && (head[len(head)-1] == '\n' || head[len(head)-1] == '\r') {
			head = head[:len(head)-1]
		}
	}

	runner := gitclient.NewExecRunner()
	registry := gitsession.NewRegistry(runner)
	t.Cleanup(registry.Close)
	discovery := gitclient.NewDiscovery(alwaysGitLocator{}, runner, gitclient.NewRealClock())
	router := New(Deps{Discovery: discovery, Runner: runner, Registry: registry, ServerVersion: "test"})
	ctx := context.Background()
	_ = router.handleAppInit(ctx)

	conn := gitsession.NewConn(gitsession.ConnID("stash-rpc-global-preflight-conn"), "test-client", "test-label", nil)
	t.Cleanup(conn.Close)
	handlers := router.ForConn(conn)

	openParams, _ := json.Marshal(RepoOpenParams{Path: dir})
	openResultAny, err := handlers.Request(ctx, "repo.open", openParams)
	if err != nil {
		t.Fatalf("repo.open: %v", err)
	}
	repoID := openResultAny.(RepoOpenResult).Repo.RepoID

	popParams, _ := json.Marshal(PreflightStashPopParams{RepoID: repoID, SHA: sha, Scope: "global"})
	resultAny, err := handlers.Request(ctx, "preflight.stashPop", popParams)
	if err != nil {
		t.Fatalf("preflight.stashPop: %v", err)
	}
	result, ok := resultAny.(gitpreflight.StashPopPreflight)
	if !ok {
		t.Fatalf("preflight.stashPop result = %T, want StashPopPreflight", resultAny)
	}
	if result.StashSha != sha {
		t.Fatalf("StashSha = %q, want %q", result.StashSha, sha)
	}
	if result.TargetSha != head {
		t.Fatalf("TargetSha = %q, want the current HEAD %q", result.TargetSha, head)
	}
}
