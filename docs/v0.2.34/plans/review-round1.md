# v0.2.34 review round 1

Reviewer: one Opus subagent, review only. Fixer: one Sonnet subagent, after this file is committed.

## Base

`a4740d063` (merge of chapter v0.2.30, PR #25). Last review session in `docs/HISTORY.md` is v0.2.30's
("Review: 7 findings ... all fixed"). v0.2.31 (P505), v0.2.32 (P506) and v0.2.33 (P507) shipped with
"No review" / "No code review (user waived)", so their diffs (`a4740d063..a84b99119`: fake agent hold
file, sorted removed-entry order, Monaco and ADE palette retune, window colours) are in scope too.
Bulk of scope: this chapter, `a84b99119..HEAD` (P508, P509 Part 1, P509 Part 2, P510, P511, full-suite
fixes `7ad065793`, `b5c454bf0`, `e544790de`, `edb5cc636`, `755b0d18a`).

Discovery: `codegraph_explore` first; index was stale for chapter files, so changed files were then read
in place.

## Findings

Count: 0 High, 2 Medium, 4 Low.

### M1 Medium: website reads a deleted token

- Where: `site/src/styles/theme.css:12`, `site/src/styles/docs.css:15`.
- Defect: P508 deleted `--kira-border-strong` from `packages/theme/src/tokens.css`. The site imports that
  file (`theme.css:1`) and still maps `--color-border-strong: var(--kira-border-strong)` and
  `--sl-color-gray-5: var(--kira-border-strong)`. No definition remains anywhere in the repo.
- Failure: every `border-border-strong` on the site (`AppHero.astro:16`, `AppTabs.astro:16`,
  `Install.astro:23,25`) and Starlight's gray-5 resolve to an invalid value at computed time. `border-color`
  falls back to `currentColor`, so those borders render in the text colour on the dark site; Starlight
  surfaces using gray-5 lose their colour. No lint or suite covers `site/`.
- Fix: map both to `var(--kira-border)` (the P508 replacement), or swap the site's classes to
  `border-border`. Add `site/` to the retired-name `check_alias` scan, or grep it in a token check.

### M2 Medium: session sandbox detail never refetches

- Where: `apps/kira-space/frontend/src/ade/v2/queries.ts:285-294` (`useSessionSandbox`),
  `apps/kira-space/frontend/src/ade/v2/readSignals.ts:34-36`.
- Defect: the client default is `staleTime: Infinity` (`packages/workbench/src/state/queryClient.ts`);
  reads rely on push invalidation. Run sandbox detail is invalidated on each runs push
  (`readSignals.ts:41`). The session key `[...runSandboxKey, 'session', id]` is invalidated nowhere.
- Failure: open the manual session's sandbox popover once (0 denials). Denials arrive; the chip from the
  sessions list reads "3 blocked" while the popover still lists none. After a Take over the row is
  replaced (`Upsert` resets policy and denials) but the popover keeps the first launch's policy and
  denials for the life of the component.
- Fix: in `onSessions`, also `invalidateQueries({ queryKey: [...runSandboxKey, 'session'] })` (prefix),
  or per session id whose badge changed.

### L1 Low: editing a stored link to the task's own Jira or host link hides the row

- Where: `apps/kira-space/internal/ade/links.go:231-249` (`editStoredLocked`, URL branch and the
  words-only tracker branch above it); `apps/kira-space/frontend/src/ade/v2/details/linkGroups.ts:89,91`.
- Defect: the duplicate check compares only other stored rows. It never compares the task's
  `JiraURL`/`JiraKey`/`HostURL` slots, which `fileLinkLocked` treats as `known`. `linkGroups` drops any
  Related row whose URL equals a Main URL, and any issue row whose ref equals the Jira key.
- Failure: a Related row edited to the task's host URL (or to its Jira key or Jira URL) saves, then
  vanishes from the Details tab. It still counts toward `MaxArtifactsPerTask`. It reappears only after the
  slot is cleared, and it cannot be removed from the UI meanwhile.
- Fix: in `editStoredLocked`, refuse with `linkErrAlreadyUsed` when the new URL matches `HostURL` or
  `JiraURL`, or when a tracker ref equals `JiraKey` (or delete the row and return `Filed: known`).

### L2 Low: a bare issue key added twice makes two rows

- Where: `apps/kira-space/internal/ade/links.go:90-121` (`fileLinkLocked`),
  `apps/kira-space/internal/ade/artifacts.go:440-445` (`storeCleanLocked`).
- Defect: dedupe is by URL only. A words-only key (`PROJ-2`) on a task whose Jira slot already holds
  another key files a stored row with `url=''`, `ref='PROJ-2'`. The unique index is `WHERE url <> ''`.
- Failure: typing `PROJ-2` into the add line twice gives two identical issue rows in Related. Pasting the
  `PROJ-2` browse URL afterwards adds a third.
- Fix: in `fileLinkLocked`, treat an existing row with the same kind and ref (url-less, or same tracker
  key) as `known`.

### L3 Low: replacing a live TUI sandbox under the same record id deletes the new one's files

- Where: `apps/kira-space/internal/ade/sandbox_tui.go:134` (scratch is `sandbox/<recordID>`),
  `:181-206` (`holdTUISandbox` / `endTUISandbox`).
- Defect: a Take over resumes with `recordID = rec.ID`, so the new sandbox's scratch path, `profile.sb`
  and proxy registration share the old one's key. When `holdTUISandbox` finds a previous live entry, it
  calls `prev.sr.cleanup()`, which runs `Release(recordID)` and `RemoveAll(scratch)`. Both hit the
  sandbox that was just prepared. The P511 plan says "same record id, new scratch"; the code reuses it.
- Failure: reachable only when the previous launch's release has not run yet. For example, a Take over
  lands between `MarkStopped` and the `OnStopped` dispatch (`tracker.go:668-680`), or a later path that
  skips `OnStopped`. The resumed agent then starts with `-f <deleted profile.sb>` and a missing `TMPDIR`,
  and with network off its proxy registration is gone.
- Fix: key scratch by launch, not by record (for example `sandbox/<recordID>-<newID>`, with the proxy and
  watch on that id). Or release the previous entry before `prepareTUISandbox` runs.

### L4 Low: workflow graph hides a confined manual session's sandbox chip

- Where: `apps/kira-space/frontend/src/ade/v2/workflows/AdeStepNode.vue:143`.
- Defect: the chip condition still excludes `kind === 'manual'`. That dates from before P511, when manual
  steps had no sandbox. Manual session steps now take one.
- Failure: a manual session step set to `write` or `read` shows no chip on its graph node. Agent and
  script steps with non-default policies do show one.
- Fix: drop the manual exclusion. `isDefaultSandbox('manual', …)` already hides the default `off`; keep it
  hidden for manual steps without a session.

## Dimensions with no findings

- Security: none found. Interactive wrap words are all `loginshell.Quote`d (`claude/headless.go:93`,
  `opencode/interactive.go`). Script wrap is argv, no shell. Link parsing admits only http(s) with a
  host. The bridge bounds link text and refuses control characters; `EditLink` slot, id and kind are
  validated. `ReadFiles` go through `real()` and `lit()`; the `path_escaping` golden covers SBPL quoting.
- Migrations: 0067 rebuild is safe (no table references `ade_task_artifacts`; the copy only widens the
  check). 0068 and 0069 snapshot rewrites touch only script and manual steps, which never applied their
  old default. `decorateTask` rehashes the re-marshalled snapshot, so the rewrite does not flag tasks as
  outdated.
- `off` semantics: agent `off` stores a row and `applySandbox` returns the spec untouched. Script
  `prepareScriptSandbox` and manual `manualSandboxPlan`/`prepareTUISandbox` return nil before any policy,
  wrapper or row exists.
- Leftovers: no references to `AddArtifact`, `UpdateArtifact`, `UpdateArtifactTitle` or
  `backend.space.ade.artifactTitle` remain outside docs. No retired class names (`rounded-kira-xs`,
  `shadow-kira-dialog`, `bg-hover`, `border-border-strong`) remain in `apps/` or `packages/`. The only
  retired token left is M1, in `site/`.
- Performance: none found. The denial watcher is one shared loop with an idle stop. Session watches stop
  in `OnTUIStopped`, and flush timers stop on flush. The row grid renders short, bounded lists. The
  per-task workflow rehash in `decorateTask` is one marshal plus SHA-256 per board build, negligible.
