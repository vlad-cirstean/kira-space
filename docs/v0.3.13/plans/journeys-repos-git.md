# P627 journeys: repositories, Git module, graph, operations, workspace, code hosts, review

Imagined usage and definition of correct. No implementation. Conventions: see `journeys-studio-db.md` head. Inputs:
`inventory.md` sections 19-23; `coverage-map.md` (gitflow, repoflow, githostflow, reviewflow, windowflow).

Far ends: `Repo.Git(...)` on local repo, `Bare.Git(...)` on bare remote, files on disk, `fakehost.Forge.Calls()`
then second read of host state, `Browser.Opened()`, `FileOpener`, op log rows, clipboard spy.

## A. Repositories and panel

### J-REPO-001 Import, scan, hide, reorder, restart

- Status: partial (`repoflow` ImportViaFolderPicker, ImportEdgeCases, HideRepo; `journeyflow` FirstRunJourney).
- Persona: developer setting up Kira on existing `~/code` with 40 repos.
- Steps:
  1. Import single repo via folder picker. Add scan folder `~/code` (40 repos, 3 linked worktrees, 1 bare, 1
     submodule, 1 broken `.git` file, symlink loop).
  2. Hide 5 repos; while scan still runs unhide one.
  3. Drag reorder; set colour on two; restart.
  4. Outside Kira: `git clone` new repo into `~/code`, delete another repo folder.
- Oracle: repo list equals `find ~/code -name .git` minus hidden, worktrees nested under parent never top-level;
  unhidden repo stays visible after scan end (`hidden_rev`); order, colours, hidden set equal after restart; new
  clone appears via watch, deleted folder leaves list or shows missing state without crash; Git and ADE pickers
  list same repos.
- Variations: path with spaces, unicode NFD (macOS) vs NFC, `? # %`; repo inside repo; 2 scan folders overlapping.
- Risk: duplicate worktree listing (P84), re-hidden on scan (P277).

### J-REPO-002 Repo config through form and YAML, used by ADE

- Status: partial (`repoflow` RepoConfigFile, RepoConfigForm; ui `ade-repos`, `repo-config-*`).
- Steps:
  1. Repositories page: nickname, prepare script `npm ci` with timeout 60 s, integration branches `main`,
     `release/*`, deployment environment "staging" with deployed-SHA Runs script ref, worktree base path, review
     base candidates.
  2. Switch to YAML tab, edit timeout, save; edit same file outside concurrently (hash conflict).
  3. Create ADE task branch in this repo: worktree lands under base path, prepare runs.
  4. Remove repo from Kira; re-import.
- Oracle: `~/.kira-space/repos/<stem>.yaml` content equals form; conflict refuses overwrite; worktree path under
  configured base (`git worktree list`); prepare script ran in worktree cwd (marker file); after remove sessions
  closed (`RemoveRepoClosesSessions`), config file kept or removed per contract (pin), re-import reuses config
  (assumption: keyed by stem/origin).
- Variations: environment with empty script (must save, P222); smart script ref refused; timeout exceeded shows
  reason on task card.
- Risk: env add always failing, config lost on re-import.

### J-REPO-003 Credential prompt relay with no focused window

- Status: partial (`gitflow` credential prompt/routes/relay; ui `git-credential-relay`).
- Steps: fetch from HTTP remote needing auth while all windows hidden; prompt opens a window; type wrong password
  (retry), then Cancel; fetch again with right password containing `'` and `\`; auto-fetch later.
- Oracle: bare remote access log shows auth attempts; cancel ends op `cancelled` not hang; password never in op
  log/app log; auto-fetch stops after credential error (no prompt storm), resumes after manual success.
- Risk: askpass quote injection, prompt storm.

## B. Git graph

### J-GRAPH-001 Browse 200k-commit history end to end

- Status: partial (`gitflow` graph paging; ui `repo-graph-*`; perf graph-scroll).
- Persona: developer hunting when a file changed.
- Pre: `NewHistory` 200k commits, 300 branches, merges, octopus merge, root commits x2.
- Steps:
  1. Open graph; scroll fast to bottom of first page; Load more until exact page multiple end ("Load the last N"
     never 0).
  2. Click commit: detail panel; Show more on long message; files tree; Go to file; Open in graph from detail.
  3. Switch to file tab and back: graph keeps scroll, selection, loaded pages (`keepMounted`).
  4. Resize columns to minimum, widen, restart: widths kept.
  5. Collapse/expand run of non-checked-out branches; `+N` chip opens refs.
- Oracle: row count and order equal `git log --date-order --all` (shas list) for loaded range; no blank rows at
  any scroll offset (every rendered row has sha); root commit tree renders; widths persisted.
- Variations: new commits arrive during scroll (collapsed counts update); checkout mid-stream; branch with
  malformed tip; commit subject 5,000 chars, emoji, RTL; author with `<` `>`.
- Risk: blank/vanishing rows (P203, P220, P225, P278), Load more exhaustion.

### J-GRAPH-002 Search then act on result

- Status: partial (`gitflow` search).
- Steps: search `fix(api)` (regex chars), by author, by sha prefix, by `#123` PR number, by PR title; Enter jumps;
  next/prev; clear; search while Load more in flight; catastrophic regex.
- Oracle: match set equals `git log --grep` / author filter over loaded plus tail scan; jump selects right sha;
  JS vs RE2 dialect difference reported not silently different; bounded time.
- Risk: regex dialect mismatch.

### J-GRAPH-003 Refs popover as daily switcher

- Status: partial (ui `repo-refs-popover`).
- Steps: open popover; type `feat/`; pin 2 branches; tabs Tags, Stashes, Worktrees, Stacks; keyboard pick branch
  and checkout with dirty tree (auto-stash on, then off); pick tag (detached); pick worktree (opens it).
- Oracle: `git rev-parse --abbrev-ref HEAD` equals pick; with auto-stash on `git stash list` gains entry and
  popover Stashes shows it; with opt-out checkout refused when conflicting, tree untouched; pins persist restart.
- Risk: auto-stash opt-out ignored (P168).

### J-GRAPH-004 PR badges follow host state

- Status: partial (`githostflow` ChangeRequestIndex*; ui `repo-graph-change-request-index`).
- Steps: open repo with 300 PRs (backfill resumable); quit mid-backfill; restart; PR merged on host; branch deleted;
  new commit pushed to PR head; rate limit hit; account switched in `gh`.
- Oracle: badge only on current PR head sha; after merge state updates on focus regain; index rows equal fake
  host PR list after sync; rate-limited state shows notice, no hammering (`Forge.Count()` bounded); account
  change triggers re-backfill.
- Risk: stale "no PR", badge on non-head.

## C. Git operations

### J-GIT-001 Feature day: branch, commit, stash, rebase-pull, push

- Status: partial (`gitflow` ops, remote; mostly per-op). New: chained with remote oracle and restart.
- Persona: developer on shared branch.
- Pre: bare remote, clone, teammate clone pushing in between.
- Steps:
  1. Create branch from graph commit; edit file in code workspace (Edit, Cmd+S); commit (staged subset).
  2. Teammate pushes to `main`. Fetch; ahead/behind chips. Pull with rebase on `main` checkout (dirty: auto-stash).
  3. Stash WIP with untracked; pop prediction says conflict; apply on other branch.
  4. Push feature; teammate force-pushes it; push again (rejected non-ff); force-with-lease (refused: lease stale);
     fetch; force-with-lease succeeds.
  5. Restart mid-sequence after step 3.
- Oracle: `Bare.Git rev-parse` after each push equals local or stays teammate's when refused; rebase result
  linear (`git log --graph`); stash list count; pop prediction equals actual `git stash pop` result on scratch
  clone; undo slot restores pre-op HEAD exactly (`rev-parse`, `status --porcelain` equal).
- Variations: protected branch `main` (push/checkout gates by upstream name); `pull.rebase=merges` in config;
  detached HEAD; upstream gone.
- Risk: lease guard, protected gate by local name, undo slot cleared early.

### J-GIT-002 Conflict mid cherry-pick, continue/skip/abort, quit in between

- Status: partial (`gitflow` cherry-pick; restack conflict).
- Steps: cherry-pick 3 commits, second conflicts; banner Continue disabled until resolved; resolve in workspace
  editor; Continue; third conflicts: Skip; start revert that conflicts: quit app; relaunch; banner present; Abort.
- Oracle: `git status` state files (`CHERRY_PICK_HEAD`, `REVERT_HEAD`) match banner at each point incl after
  relaunch; final log has commits 1, 2 only; abort returns HEAD and tree to pre-revert.
- Variations: merge in progress started in terminal (Skip not offered); rebase started outside Kira.
- Risk: banner state desync after restart.

### J-GIT-003 Reset three modes and undo

- Status: partial (`gitflow` matrix undo slot).
- Steps: with staged, unstaged, untracked changes: reset soft to HEAD~2; undo; reset mixed; undo; reset hard
  (confirm names loss); undo.
- Oracle: after each undo `rev-parse HEAD` equals original; after hard reset undo restores HEAD but working
  changes: assumption lost (confirm said so) unless auto-stash kept them (pin); untracked files never deleted.
- Risk: undo claims more than it restores.

### J-GIT-004 Worktrees from graph and panel

- Status: partial (`gitflow` worktree add/remove; `adeflow` multiworktree).
- Steps: "Create worktree here" on commit; prepare script progress then failure; retry; switch worktree; open
  terminal on worktree row; try to check out branch already checked out elsewhere; remove dirty worktree
  (confirm); remove worktree whose folder was deleted outside.
- Oracle: `git worktree list --porcelain` after each; refused checkout names other worktree; terminal cwd equals
  worktree path (`pwd` output); prune of missing folder works.
- Risk: prepare incomplete on wait delay.

### J-GIT-005 Stacked branches restack

- Status: partial (`gitflow` stack restack, conflict).
- Steps: stack A <- B <- C; amend A in terminal; Stacks tab shows needs restack; Restack; conflict in B; resolve;
  continue; push all with force-with-lease.
- Oracle: `merge-base` relations: B on new A, C on new B; remote tips equal local; untouched branch D unchanged.
- Risk: partial restack leaves C on old B silently.

### J-GIT-006 Remote failure matrix

- Status: partial (`gitflow` remote cancel, BadRemote, matrix).
- Steps: fetch with remote unreachable (banner, Retry); slow remote: Cancel at 2 s; repo folder deleted while
  fetch runs; two windows fetch same repo at once; auto-fetch backoff over 5 failures; network back.
- Oracle: no orphan `git` process (process table); op rows end states; second window joins or queues (one fetch
  observed in remote access log, assumption singleflight); auto-fetch resumes after success, not disabled for
  life.
- Risk: circuit breaker tripped by cancels, auto-fetch dead forever.

### J-GIT-007 Date format and op log for every spawn

- Status: covered (`appflow` DateFormatReachesGit, OpsEveryGitSpawn); extend: copy.
- Steps: change date format in Settings; reopen graph and blame; open Ops panel; Copy details of a git row; Copy
  command.
- Oracle: dates in graph/detail/blame use format; clipboard command runnable in shell, URL credentials redacted.
- Risk: credential in copied command.

## D. Code workspace and files

### J-WS-001 Read, search, quick open, edit, conflict

- Status: partial (`repoflow` BrowseReadDiff, CodeSearch, EditFile; ui `repo-workspace`, `repo-file-edit`). New:
  hash conflict far end (coverage gap).
- Persona: reviewer making a quick fix.
- Steps:
  1. Cmd+P `conf yml`; open file; tree shows tracked plus untracked not ignored.
  2. Repo search `TODO\(` streaming over 50k files; click result opens at line; close workspace mid-search.
  3. Edit worktree file (CRLF, BOM, mode 755): press Edit, change line, Cmd+S.
  4. Outside: change same file; edit again in Kira; Cmd+S: conflict Refresh/Overwrite; choose Refresh; redo;
     outside change again; Overwrite.
  5. Try editing 9 MiB file (cap), binary file, file in revision-pinned tab (read only), symlink.
- Oracle: disk bytes after save: only changed line differs, BOM/CRLF/mode kept (`stat`, `xxd` head); Refresh
  loads external content, no write; Overwrite writes Kira buffer; search process ended on close (no `git grep`
  left); atomic write (no temp file left, inode replaced).
- Variations: file deleted outside while dirty; file renamed; unicode filename NFD; quit with dirty buffer
  (accepted loss, no prompt).
- Risk: EOL/BOM loss, overwrite without conflict.

### J-WS-002 Diff views and navigation

- Status: partial (ui `repo-workspace`).
- Steps: change 12 files (rename, binary, mode-only, deleted, 20k-line); "Open all changes"; next/previous change
  crosses files; toggle side-by-side/inline (persists per device); copy selection from diff right side; stage
  nothing; commit detail file diff from graph then back (detail still shown).
- Oracle: file list equals `git status --porcelain` incl rename pairing; clipboard equals working tree lines
  selected (no diff markers); layout key `kira.repo.diffLayout` persists; hidden tab 0x0 measure does not blank
  detail.
- Risk: commit detail vanishing (P343).

### J-WS-003 Blame through history

- Status: partial (`gitflow` blame).
- Steps: toggle inline blame on 5k-line file; move cursor: status bar item author/age/subject; click reveals
  commit in graph; file with uncommitted lines; file renamed in history; revision-pinned tab (never blamed).
- Oracle: blame per line equals `git blame --porcelain`; uncommitted lines marked not-committed; reveal selects
  right sha; fast cursor moves do not show stale line's blame.
- Risk: stale nav status after fast switch.

### J-WS-004 External opens

- Status: partial (`windowflow` OpenedFilesWindow, TerminalFileLinks; `appflow` filehandlers).
- Steps: Finder-open 5 files within 300 ms (2 in known repo, 3 outside); terminal link `src/a.ts:12:5` relative to
  live cwd after `cd`; path with spaces and `:` ; nonexistent file; open same file twice.
- Oracle: one files window for the burst; repo files open as repo-file tabs with cursor at 12:5, outside files
  read-only local; second open focuses existing tab; missing file shows error not blank tab; files window not
  restored after restart.
- Risk: window per file, wrong cwd resolution.

## E. Code hosts and issue tracker

### J-HOST-001 PR from link to comment round trip

- Status: partial (`githostflow` hostview, comments, githublinks; ui `host-view*`).
- Persona: reviewer gets PR URL in chat.
- Steps:
  1. Paste `https://github.com/o/r/pull/12/changes` in capture or click in commit body: opens host-view window.
  2. Read description (images as links), checks, comments (capped note). Refresh.
  3. Write comment with Markdown (code fence, `@mention`, emoji, 20 KiB), post; reply to thread; post line comment
     after head moved (refused with head-moved notice).
  4. Copy link; open on host (browser).
- Oracle: `Forge.Calls()` has one post per action with exact body bytes; second fetch returns the comment and UI
  shows it once (no duplicate optimistic row); head-moved post not sent; clipboard URL canonical;
  `Browser.Opened` URL.
- Variations: GHES host; Azure Repos; fork PR; `gh` missing (CLI tools pane guidance), unauthenticated; `gh`
  output 10 MiB; sub-path links `/files`, `/commits/sha`, trailing slash; non-PR link falls back to browser.
- Risk: link misclassification (P594), duplicate post on retry, truncated `gh` output.

### J-HOST-002 Jira issue view with cache and offline

- Status: partial (`githostflow` issueview, issueview_fetch; `adeflow` TaskIssueTracker).
- Steps: open issue `ABC-12` (ADF with table, callout, code, mention); comment; reopen within 30 min (cache, then
  revalidate); acli offline (stale shown with age); batch of 60 tracked issues (two searches of 50 max); issue key
  in other project/case.
- Oracle: fake acli call count: one read then one revalidate, batch split 50+10; posted comment body ADF/markdown
  equals input; stale-while-revalidate never shows other issue's data; save racing in-flight read does not
  resurrect old detail (P470).
- Risk: cache key mix-up, detail save race.

### J-HOST-003 CLI tools pane guided setup

- Status: partial (`appflow` CliTools; ui `settings-cli-tools`).
- Steps: no `gh`: pane shows install step; copy command; install fake `gh`; Recheck; login step; `acli` Homebrew
  tap trust; `az` extension missing.
- Oracle: clipboard commands exact and safe; status flips without restart; Kira never reads token files (file
  access audit on fake HOME).
- Risk: credential touch.

## F. Code review window

### J-REV-001 Incremental review across rebases

- Status: partial (`reviewflow` incremental marks, ranged walk; `adeflow` review_*; ui `ade-v2-review*`).
- Persona: reviewer over three days.
- Steps:
  1. Open review window for branch (20 files). Mark 8 files reviewed, mark range of hunks in one, tri-state a
     folder. Alt+F8 jumps to next unreviewed.
  2. Author rebases onto new main and amends 2 reviewed files, deletes one reviewed file. Fetch in review header.
  3. "Since review" view: only real changes; removed-since-review section lists deleted file; dismiss.
  4. Close window, quit, reopen review.
- Oracle: marks stored by content snapshot: unchanged files stay reviewed after rebase (sha changed), amended files
  flip to changed with diff equal `git diff <snapshot-blob> <new-blob>`; state after restart equal (`review.db`);
  review windows not auto-restored but state is.
- Variations: uncommitted work reviewed then committed (promote); file over patch cap (open item: pin); branch
  renamed (open: old rows); 2,000-file diff; binary and renamed files.
- Risk: review stale on new commit (P384), `changedSinceReview` ignored.

### J-REV-002 Comments to agent and to PR

- Status: partial (`reviewflow` comments export; `adeflow` ReviewForwardComments; `githostflow`
  ChangeRequestReviewActions).
- Steps:
  1. Add 6 comments (kinds change request/question) on lines incl multi-line range, one on removed line, one on
     file-level; edit one; delete one.
  2. Copy as prompt. Send review comments 5 to agent (running TUI session). Sent ones cleared.
  3. Add 2 more; submit PR review "Request changes" with summary (resizable dialog); Approve later.
- Oracle: clipboard prompt lists 5 comments with file:line and text in file order; agent PTY received same text
  (fake agent record); `Forge.Calls()` review with event REQUEST_CHANGES and line comments mapped to right
  side/line; second fetch shows them; no duplicate on double-click Submit.
- Variations: PR head moved before submit; foreign PR by number; ref spelling `origin/feat/x`, fork
  `owner:branch`; no PR (actions hidden with reason).
- Risk: line mapping wrong side, missing actions for ref spelling (P392).

### J-REV-003 Review header freshness

- Status: partial (`adeflow` ReviewSync, ReviewSyncStackedBase; ui `ade-v2-review-sync`).
- Steps: badges Diverged / not pushed; Fetch; Update branch (ff-only) refused when diverged; Update base; head
  badge "At <sha>" then Stale after remote push; stacked base moves.
- Oracle: local refs after each action per `git rev-parse`; refused update changes nothing; diff base equals
  merge-base shown.
- Risk: stale head shown as current.

### J-REV-004 Diff ergonomics with keyboard

- Status: partial (ui `repo-review-interaction`, `ade-v2-review-follow`).
- Steps: navigate files with keys; file list follows shown file; expand unchanged region; comment line inside
  collapsed region stays revealed; narrow window (side-by-side stays); select text across hunk, copy; thread
  overlay capped height scrolls.
- Oracle: clipboard equals selected new-side text; revealed regions include every commented line.
- Risk: comment hidden in collapsed region.
