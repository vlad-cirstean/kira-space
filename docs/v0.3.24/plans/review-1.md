# v0.3.24 code review round 1

Base: `023636358` (docs: record v0.3.23 review round 4 fixes, the last commit of the last review
session). Head reviewed: `f783682ea`. Diff: `git diff 023636358 f783682ea`. It holds the v0.3.23
post-review leftovers folded into the squash `b22e50942` (UI spec updates for the kept-details hover,
`internal/fswatch/scope.go` `forget(key, unwatch)`) plus P736-P744 and the full-run commit.

Three dimensions: (1) architecture, structure, maintainability, security; (2) functional correctness
and business logic; (3) performance and resource use.

Counts: must-fix 0, should-fix 2, nit 2.

## Findings

### R1-1 should-fix (1, security/robustness): rtk hook is not fail-open on a non-zero exit

`.claude/hooks/rtk-pretooluse.sh:5-7`

Evidence: the wrapper's only guards are `command -v rtk` and `rtk pipeline --help` (full edition).
Then `exec rtk hook claude` hands rtk's exit code straight to Claude Code. A PreToolUse hook exiting
2 blocks the tool call. Any `rtk` on PATH that is not the full edition passes the guard. That
includes a foreign binary named `rtk`, which `scripts/rtk-setup.sh` itself names (the crates.io
`rtk`), and an rtk-oss build older or newer than the pinned `1.2.1` that lacks `hook claude`. A clap
usage error exits 2, so every Bash call in every session in this repo would be blocked. The header
says "Fails open"; that holds only for the cases rtk itself handles. The probe also inherits the
hook's stdin.

Fix: do not `exec`. Check `rtk --version` equals `rtk 1.2.1` (or at least starts `rtk `), as setup
does. Run the probe with `</dev/null`. Run `rtk hook claude`, capture stdout, and on any non-zero
exit print nothing and `exit 0`. Print the captured stdout only on exit 0.

### R1-2 should-fix (2, correctness): collapsed uncommitted section stays shut while the cursor steps inside it

`packages/git-ui/src/components/review/ReviewFilesPane.vue:126-132`

Evidence: `workingOpen` reopens only on a change of `props.cursor?.section` to `'working'`. Collapse
the section while the cursor sits on an uncommitted file, then step to the next uncommitted file or
follow a centre tab onto one. The section stays `'working'`, the watch does not fire, and the cursor
lands on a hidden row. The comment above the watch states that case must not happen; P740 Result
says the section "reopens when the cursor moves onto a working file".

Fix: watch the cursor position, not only its section, e.g. `() => [props.cursor?.section,
props.cursor?.path] as const`, and reopen when the section is `'working'`. Add the step-inside case
to `ade-v2-review-working.spec.ts`.

### R1-3 nit (2, correctness): proof window opens full size when no window to cascade from

`apps/kira-space/main.go:49-55`

Evidence: `openProofWindow` sets `proof.WindowWidth`/`WindowHeight` only when
`shell.CascadeFrom(app.Window.Current())` returns bounds. `CascadeFrom` returns nil when there is no
current window, which its own doc says is always the case on Linux. Then `OpenWindow` gets no bounds
and the proof window opens at the 1280x800 default, missing P736's compact window.

Fix: when `rec.Bounds` is nil, set `&shell.WindowBounds{Width: proof.WindowWidth, Height:
proof.WindowHeight}` with an origin `Options`/`EnsureOnScreen` accepts (primary work-area origin).

### R1-4 nit (3, performance): `markChunk` locks paths it does not process

`apps/kira-space/internal/gitsession/incremental.go:786-813`

Evidence: a chunk takes the per-path locks of all its window (up to 256 paths) before preparing, but
the 16 MiB byte budget can stop it after a few paths. The unprocessed paths stay locked through the
chunk's git reads and `PutMany`, so a concurrent `review.mark` on any of them waits for work that
never touches it, and the next chunk locks them again. Large files make this the common shape.

Fix: process the window in sorted path order and take each path's lock just before its
`prepareMark` (sorted order still rules out a cycle between two concurrent `MarkFiles`; other
holders take one lock). Locks then cover only consumed paths. `recs` is keyed by path, so result
order is unaffected.

## Probed, nothing found

- P742 `confirm`: refusal paths run before the confirm gate in every tool (`gateMode`, explain
  preflight deny, kube `Approval` fail-closed, `needsAPIApproval`, script policy); confirm only adds
  a card. `holdResult` clears `AgentNote` and the confirm reason for every held result (db, kube,
  api). The note reaches no audit row, op log or prepare script; it renders as interpolated text,
  never HTML. Cap and trim checked first; whitespace-only means none. Flow test covers deny, cap and
  held-result cleanliness.
- P739 `MarkFiles`: sorted lock order is deadlock-free against single-lock holders and concurrent
  batches; `PutMany` is one transaction with deferred rollback; `CaptureRemoved` is idempotent, so a
  failed chunk after captures retries cleanly. Flate writer pool resets per use. Virtual `FileTree`:
  `navRows`/`selectRow` indexes agree in both modes, reveal goes through `revealKey`, capped path
  keeps its page, announce and kept scroll. `ReviewFilesState.changes` updates only on list change;
  marks replace `files` only.
- P740 deleted view: teardown disposes widget, registry entry and commands before the remount;
  `registerEditor`/`registerDiffEditor` release the unused model uri when switching widget kind.
- P743: tracker row still listed (static row); Jira failures surface as a gap from `TaskIssue`
  (`noTracker` excluded); the pending dot cannot stick, since the Jira item and the issue query's
  `enabled` both key on `task.jira`. `artifactLess` is a total order.
- P744 setup: pinned SHA-256, exact archive contents, edition and version checks, quoting sound,
  telemetry off; local `~/.local/share/rtk` data documented in `docs/DEV_ENVIRONMENT.md`.
- v0.3.23 leftovers: `fswatch` `forget(key, unwatch)` keeps live watches on Create only; Remove and
  Rename still unwatch. Spec edits follow the kept-details hover.
- No `.vue` over 600 lines (`FileTree.vue` 599). i18n keys added are used. No new test merely
  restates code; perf probes are gated (`skipUnlessPerf`, perf Playwright config).
