# P627 journeys: Git and repos, second pass

Continues `journeys-repos-git.md` (ids continue). One scenario per journey. Conventions: `journeys-studio-db.md`
head. Far end: `Repo.Git(...)`, `Bare.Git(...)`, files on disk, `git config --get`, `git reflog`, `git stash list`.

Code facts used (CodeGraph reads):

- `gitsession/ops.go:350-430` `prepareCheckout`: `AutoStash` plus `DiscardLocalChanges` together refused, no write;
  in-progress operation re-checked host-side (`OperationInProgress`); dirtiness recomputed at write time, clean
  tree skips stash; stash message "auto-stash: switching to <target>"; stash never popped back automatically;
  target kind tag or sha detaches; remote branch creates local tracking branch.
- `ops.go:802-881` `prepareReset`: mode must be `soft|mixed|hard`; in-progress refuses; unresolvable target
  `NotFound`; hard reset with anything to destroy needs `ConfirmToken` equal to target short sha, recomputed fresh;
  undo record holds pre-write HEAD, label `Reset (<mode>) to <subject>`; hard undo replays as `--keep`; no undo on
  unborn HEAD.
- `ops.go:887-920` `prepareCherryPick`: in-progress refuses; undo withheld for `NoCommit` and unborn HEAD; undo
  replay `reset --keep`.
- `ops.go:468-477, 1002-1036` branch delete: undo captures tip sha, every `branch.<name>.*` config line and each
  stack child's prior parent/base; delete re-parents stack children.
- `ops.go:1044-1096`: tag delete undo recreates annotated tag from tag object sha; stash drop undo is
  `stash store -m <message> <sha>` (top of stack, index not guaranteed); global stash remove undo restores
  `refs/kira/globalstash/<sha>`; removal uses expected-old-value delete.
- `gitpreflight/undo.go`: one undo slot per repo (`Set`, `Peek`, `Take`); next op clears it; record carries
  `OriginConn`/`OriginLabel` (which window did it) and expected tips.
- `gitsession/worktree.go:250-392`: remove blockers `notAWorktree`, `mainWorktree`, `currentWorktree`,
  `openInAnotherWindow`, `locked` (only `locked` has kind `WorktreeLocked`); dirty remove needs typed token
  re-derived server-side, force decided server-side; add blockers `pathExists`, `branchExists`,
  `branchCheckedOutElsewhere`, `unknownStartPoint`, `invalidPath`; modes `existingBranch`, `newBranch`, `detach`.
- `gitpreflight/stack.go`: stack parent/base live in `branch.<name>.kirastackparent/.kirastackbase` git config;
  `MaxStackedBranches` 64 (beyond: listed, counts zero or orphaned by depth); dangling parent or cycle lands in
  `Orphans` with state `parentMissing`, never dropped; `behind > 0` means `needsRestack`; children sorted by name.
- `gitsession/remote.go:32-52, 690-732`: remote kinds fetch, push, pull, forcePush, deleteRemoteBranch; force push
  carries `ExpectedRemoteTip` (lease), `PlainForce`, `ConfirmToken`; protected-branch match runs against the
  remote branch name, not local name; upstream with other name resolved (`resolveUpstreamRemoteBranch`).

## H. Checkout and stash

### J-GIT-020 Switch branch with dirty tree, auto-stash, come back hours later

- Status: partial (`gitflow` TestCheckoutDirtyAutostash).
- Persona: developer interrupted mid-edit by hotfix.
- Pre: repo on `feature`, 2 tracked files modified, 1 untracked, 1 staged new.
- Steps:
  1. Pick `main` in refs popover. Preflight dialog lists dirty paths; choose stash and switch.
  2. Do hotfix on `main`: edit, commit, push.
  3. Switch back to `feature` (clean tree: no stash prompt).
  4. Open stash list; find entry "auto-stash: switching to main" tagged `On feature`.
  5. Pop it. Close app, reopen.
- Oracle: `git stash list` after step 1 has exactly 1 entry with that message; untracked file inside stash only if
  it was untracked-and-needed (`-u` derived server-side: pin rule); after step 5 `git status --porcelain` equals
  pre-step-1 status byte for byte (staged-new still staged: assumption, `--index` pop; OQ-GIT-1); stash list empty.
- Variations: step 3 with new dirt on `main` (second auto-stash, 2 entries, correct tags).
- Risk: staged state lost on pop; stash never surfaced after switch so work looks lost.

### J-GIT-021 Stale preflight: tree goes clean while dialog is open

- Status: new (host-side recompute read from code).
- Steps: 1. Dirty tree; open switch dialog (stash option preselected). 2. In terminal `git checkout -- .` and delete
  untracked file. 3. Confirm dialog.
- Oracle: switch happens; `git stash list` unchanged (no empty stash, no "No local changes to save" error);
  announcement says switched, no stash mention.
- Risk: confusing no-op stash error.

### J-GIT-022 Stale preflight: merge starts while dialog is open

- Status: new.
- Steps: 1. Dirty tree; open switch dialog. 2. In terminal start merge that conflicts. 3. Confirm with stash.
  4. Confirm with discard instead.
- Oracle: both refused with `OperationInProgress` naming merge; no stash entry, `MERGE_HEAD` still present, HEAD
  unchanged; banner offers continue/abort.
- Risk: stash push mid-merge with empty stderr misclassified; discard destroys merge state.

### J-GIT-023 Checkout tag, sha and remote-only branch

- Status: partial (`gitflow` checkout).
- Steps: 1. Checkout tag `v1.0`. 2. Checkout sha typed by hand (7 chars). 3. Checkout `origin/topic` (no local).
  4. Checkout `origin/topic` again when local `topic` exists but diverged. 5. Type target `-f`.
- Oracle: 1 and 2 `git symbolic-ref -q HEAD` fails (detached) and HEAD equals target; 3 creates local `topic` with
  upstream `origin/topic`; 4 does not reset local `topic` (blocker or switch to existing: pin); 5 refused by
  argument validation, no git spawn.
- Risk: option injection via ref name, silent reset of local branch.

### J-GIT-024 Stash matrix: save, apply, pop with conflict, drop, undo drop

- Status: partial (`gitflow` TestStashFlows).
- Steps: 1. Save stash A (tracked), B (with untracked), C (staged only). 2. Apply B. 3. Edit same lines; pop A
  (conflict). 4. Resolve, drop A by hand. 5. Undo. 6. Do any other op, try Undo again.
- Oracle: after 3 stash A still in list (git keeps on conflict), conflict markers in file, UI says so; after 5
  `git stash list` top entry has A's message and same sha (index 0, not original index); after 6 undo slot empty
  (button disabled), no replay of stale record.
- Risk: pop conflict reported as success; undo restoring into wrong position confuses indexes in open UI.

### J-GIT-025 Global stash across two repos and removal undo

- Status: partial (inventory global stash; flows partial).
- Steps: 1. Save global stash in repo. 2. Open second window on worktree of same repo; list shows entry. 3. Remove
  it in window 2. 4. Window 1 presses Undo.
- Oracle: `git for-each-ref refs/kira/globalstash/` empty after 3, entry back after 4; undo label names origin
  window (`OriginLabel`); removing already-removed entry gives `NotFound` "no longer exists".
- Risk: undo from other window restores wrong ref or is hidden.

## I. Reset, cherry-pick, revert, undo

### J-GIT-026 Hard reset with typed confirmation and stale dialog

- Status: partial (`gitflow` TestResetAndUndo).
- Steps: 1. Dirty tree (staged, unstaged, staged-new). 2. Reset hard to `HEAD~3`: dialog lists what is destroyed,
  asks for short sha. 3. Type wrong token. 4. While dialog open, commit everything in terminal. 5. Confirm with
  correct token. 6. Undo.
- Oracle: 3 refused `ConfirmationRequired`, HEAD unchanged; 5 succeeds (tree clean now; token no longer required
  but harmless); after 6 HEAD equals commit made in step 4 and files intact (`--keep` replay never discards).
- Variations: clean tree hard reset needs no token; mode sent as `keep` or `HARD` refused (`ErrInvalidResetMode`).
- Risk: confirm token checked against stale preflight; undo of hard reset destroying new work.

### J-GIT-027 Undo after new local edits

- Status: new.
- Steps: 1. Reset mixed to `HEAD~1`. 2. Edit a file that differs between the two commits. 3. Undo.
- Oracle: defined result (OQ-GIT-2): `reset --keep` refuses when local change collides; UI must report refusal
  with reason and keep undo available or state it expired; HEAD and file unchanged on refusal.
- Risk: silent overwrite or dead Undo button without message.

### J-GIT-028 Cherry-pick series with conflict, empty pick and no-commit

- Status: partial (`gitflow` TestCherryPickRevertConflict).
- Steps: 1. Pick commit X onto `release` (clean). Undo. 2. Pick again; pick Y that conflicts; resolve; continue.
  3. Pick Z already applied (empty). 4. Pick W with no-commit. 5. Pick merge commit M with mainline 1.
- Oracle: after undo HEAD back, tree clean; empty pick surfaced as "already applied" with skip, `CHERRY_PICK_HEAD`
  gone after skip; no-commit leaves staged diff equal to W's diff and offers no Undo; mainline pick diff equals
  `git diff M^1 M`.
- Risk: empty pick shown as conflict with zero files; undo offered for no-commit.

### J-GIT-029 Delete branch that is a stack parent, then undo

- Status: partial (`gitflow` TestStackRestack; undo of re-parent new).
- Pre: stack `main` < `a` < `b` < `c`, `a` has upstream and config `branch.a.description`.
- Steps: 1. Delete `a` (force). 2. Inspect stack view. 3. Undo. 4. Inspect config.
- Oracle: after 1 `git config branch.b.kirastackparent` is `main`; after 3 `refs/heads/a` equals old tip,
  `branch.a.remote/merge/description` restored, `branch.b.kirastackparent` is `a`; stack view shows 3 levels again.
- Risk: child left orphaned; undo restores ref but not config.

### J-GIT-030 Delete annotated and lightweight tags, undo both

- Status: new.
- Steps: 1. Delete annotated tag `v2` (message, tagger). Undo. 2. Delete lightweight tag `tmp`. Undo. 3. Delete
  tag, create same-name tag at other commit in terminal, press Undo.
- Oracle: `git cat-file -t v2` is `tag` after undo with same object sha and message; `tmp` points at same commit;
  step 3 undo refused or reports conflict, never moves the new tag (OQ-GIT-3).
- Risk: annotated tag recreated lightweight; overwrite of new tag.

### J-GIT-031 Undo attribution across two windows on one repo

- Status: new.
- Steps: 1. Window A deletes branch `x`. 2. Window B looks at Undo button tooltip. 3. Window B runs fetch (not a
  ref-moving local op: pin whether slot survives). 4. Window B presses Undo. 5. Window A presses Undo.
- Oracle: tooltip in B names A as origin; exactly one undo executes (second finds slot taken, shows nothing to
  undo); `refs/heads/x` restored once.
- Risk: double replay, undo of other window's op without attribution.

## J. Worktrees

### J-WT-010 Create worktree in each mode with every blocker

- Status: partial (`gitflow` TestWorktreeAddRemove, `worktree_state_test`).
- Steps: 1. New branch `wt-a` from `origin/main` at `../repo-wt-a`. 2. Existing branch `topic`. 3. Detached at tag.
  4. Path that exists as file. 5. Branch name that exists (new-branch mode). 6. Branch checked out in main
  worktree. 7. Start point `nope`. 8. Path inside repo `.git/`. 9. Path with unicode and spaces.
- Oracle: `git worktree list --porcelain` shows 3 entries plus step 9; steps 4-8 Create disabled by preflight and,
  when forced through stale dialog, backend errors `AlreadyExists`, `AlreadyExists`, `WorktreeConflict`,
  `NotFound`, invalid path; no directory created on any refusal.
- Risk: partial directory left behind, DWIM branch creation.

### J-WT-011 Remove worktree: blockers and dirty confirmation

- Status: partial.
- Steps: 1. Try remove main worktree. 2. Try remove current. 3. Open worktree in second window; try remove from
  first. 4. `git worktree lock --reason "usb"`; try remove. 5. Unlock; make it dirty (untracked only); remove:
  token dialog; type wrong, then right. 6. Remove clean worktree.
- Oracle: 1-3 menu item disabled with reason; 4 error names lock reason; 5 wrong token `ConfirmationRequired`,
  directory intact; right token removes directory and untracked file; `git worktree list` no longer lists it;
  branch ref kept.
- Risk: remove of path not a worktree (security gate), force taken from client.

### J-WT-012 Worktree becomes dirty between dialog and confirm

- Status: new.
- Steps: 1. Clean worktree; open remove dialog (no token asked). 2. Agent or terminal writes file there. 3. Confirm.
- Oracle: refused with `ConfirmationRequired` and token; file intact; dialog switches to token form.
- Risk: fail-open delete of fresh work.

### J-WT-013 Prunable worktree after directory deleted outside

- Status: new.
- Steps: 1. `rm -rf` worktree directory in file manager. 2. Refresh worktree list. 3. Try open it. 4. Prune (if
  offered) or remove. 5. Create worktree again at same path and branch.
- Oracle: entry shown prunable with reason; open refused with clear message; after prune `git worktree list`
  clean and step 5 succeeds (branch not reported checked out elsewhere).
- Risk: stale registration blocks branch forever.

### J-WT-014 Two windows, one repo: switch worktree, detach, reattach

- Status: partial (`gitflow` worktree_state_test).
- Steps: 1. Window A on main worktree, window B on `wt-a`. 2. In A use switch-worktree to `wt-a` (already open in
  B). 3. In A switch to `wt-b`. 4. Commit in B; look at A's graph. 5. Close B; in A remove `wt-a`.
- Oracle: 2 focuses B or refuses with open-elsewhere (pin), never two sessions writing one worktree; 4 A's graph
  shows B's commit after refresh event; 5 allowed only after B closed.
- Risk: `OpenElsewhere` stale after window close.

## K. Stacks

### J-STACK-001 Build 4-level stack, amend bottom, restack, undo

- Status: partial (`gitflow` TestStackRestack).
- Steps: 1. Create `a` on `main`, `b` on `a`, `c` on `b`, `c2` on `b`. 2. Amend commit in `a`. 3. Stack view shows
  `b`, `c`, `c2` needs-restack. 4. Restack all. 5. Undo.
- Oracle: after 4 `git merge-base b a` equals `a` tip for each level, `c` and `c2` both on new `b`; order in view
  `a, b, c, c2` (sorted children); after 5 every branch tip equals pre-restack sha (`git rev-parse` list).
- Variations: conflict in `c` during restack: `b` done, `c` in progress, `c2` not started; abort restores `c`.
- Risk: partial restack undo restoring only last branch.

### J-STACK-002 Hand-edited stack config: cycle, dangling parent, 70-deep chain

- Status: new.
- Steps: 1. `git config branch.a.kirastackparent c` (cycle a-b-c). 2. Open stack view. 3. Fix; delete ref of `b`
  with `git update-ref -d` leaving config. 4. Open view. 5. Script 70 chained branches. 6. Open view.
- Oracle: no hang; 2 all three under Orphans with remedy; 4 `c` listed orphan "parent 'b' no longer exists", not
  grouped under `main`; 6 every one of 70 branches listed (stack or orphan), none dropped; view opens under 2 s.
- Risk: infinite loop, silent drop.

### J-STACK-003 Stack on remote-tracking base that gets pruned

- Status: new.
- Steps: 1. Stack `x` on `origin/release`. 2. Remote deletes `release`; fetch with prune. 3. Open view. 4. Re-parent
  `x` onto `main` via menu. 5. Undo re-parent.
- Oracle: after 2 `x` shown with missing base (orphan or base tip absent), no crash; after 4 config parent `main`;
  after 5 config parent `origin/release` again.
- Risk: nil base tip dereference.

## L. Remote operations

### J-REM-010 Push branch whose upstream has another name

- Status: new (rule read from code).
- Pre: local `fix` tracks `origin/bugfix/123`; `origin/fix` does not exist; `bugfix/*` protected pattern set.
- Steps: 1. Open push dialog. 2. Read preflight. 3. Push. 4. Force push (amend first).
- Oracle: preflight never says "will set upstream"; ahead/behind computed against `origin/bugfix/123`; bare repo
  `refs/heads/bugfix/123` advances, `refs/heads/fix` never created; force push shows protected-branch gate
  because remote name matches pattern although local name does not.
- Risk: push creating stray same-name branch; protection bypass through renamed local branch.

### J-REM-011 Force push with lease when remote moved

- Status: partial (`gitflow` remote).
- Steps: 1. Fetch. 2. Colleague pushes to same branch (bare repo updated out of band). 3. Amend locally; force push
  from dialog (lease on tip seen in step 1). 4. Read error. 5. Fetch, inspect incoming commit, force push again.
  6. Tick plain force with typed confirmation on a third round.
- Oracle: 3 rejected, bare tip equals colleague's commit; error names stale lease and suggests fetch; 5 succeeds
  with new expected tip; 6 only with token; reflog on bare shows no intermediate overwrite.
- Risk: lease using current remote-tracking value read at push time instead of what user saw.

### J-REM-012 Pull strategies with dirty tree and conflict

- Status: partial.
- Steps: for merge, rebase, ff-only: 1. Remote ahead 2, local ahead 1, dirty unrelated file. 2. Pull. 3. With
  conflicting local commit: pull rebase; resolve; continue. 4. Abort instead.
- Oracle: ff-only refuses with reason (diverged); merge creates merge commit with 2 parents; rebase gives linear
  history, local commit on top; dirty file untouched in all; abort returns HEAD and tree to pre-pull sha.
- Risk: dirty file lost by rebase autostash failure.

### J-REM-013 Fetch prune removes branch shown in open views

- Status: new.
- Steps: 1. Graph filtered to `origin/old`; review tab open on it. 2. Remote deletes branch. 3. Fetch with prune and
  prune tags. 4. Look at graph, refs popover, review tab.
- Oracle: ref gone from popover; graph filter shows "ref no longer exists" state, not empty spinner; review tab
  keeps content with gone marker; local branch tracking it shows `gone` track state.
- Risk: stale ref click errors.

### J-REM-014 Delete remote branch and credential prompt cancel

- Status: partial (`gitflow` credential_test).
- Steps: 1. Delete remote branch `origin/tmp` (confirm). 2. Remote needs credentials: prompt appears; cancel.
  3. Retry; enter wrong; enter right. 4. Close window while prompt open on another op.
- Oracle: cancel leaves bare ref intact and op ends "cancelled", not error toast; right credential deletes ref;
  window close cancels git process (no orphan `git` in process list).
- Risk: hung git waiting on prompt after window close.

## M. Long sessions

### J-GIT-040 Release day: 3-hour session across graph, stacks, worktrees and host

- Status: new.
- Persona: release owner.
- Steps:
  1. Open repo; fetch all remotes with prune.
  2. Create worktree `release-1.4` from tag `v1.3.0`.
  3. In that window cherry-pick 6 commits from `main` chosen in graph (multi-select), one conflicts; resolve in
     editor; continue.
  4. Back in main window, colleague force-pushed `main`: pull refuses ff; inspect incoming; reset hard to
     `origin/main` with token; realise local commit lost; Undo.
  5. Rebase local commit onto `origin/main` instead.
  6. In release window create annotated tag `v1.4.0`, push branch and tag.
  7. Open pull request via host panel from release branch; copy its URL.
  8. Quit app mid-push of a second tag (network cut); relaunch.
  9. Retry push; delete wrong local tag; Undo; delete again.
- Oracle: bare repo: `refs/heads/release-1.4` has 6 picked commits in chosen order with original author dates,
  `refs/tags/v1.4.0` annotated; local commit from step 4 reachable from `main` after step 5; clipboard holds PR
  URL from `fakehost.Forge`; after relaunch no stale lock file blocks ops and second tag absent or complete on
  remote, never partial; reflog shows undo as `reset --keep`.
- Risk: order of multi-select pick, lost work on reset, lock left by killed push.

### J-GIT-041 Review marathon: 400-file change request with comments, restart and force-push

- Status: partial (`reviewflow`).
- Steps: 1. Open change request with 400 files, 2 binary, 1 rename, 1 submodule bump. 2. Mark 150 files viewed,
  write 12 draft comments (3 multi-line ranges). 3. Quit; relaunch. 4. Author force-pushes (5 files change).
  5. Refresh. 6. Send review.
- Oracle: viewed marks and drafts persist restart; after force-push viewed marks cleared only for changed files;
  drafts on vanished lines flagged outdated, not dropped; `fakehost.Forge.Calls()` shows one review submission
  with 12 comments, correct paths, sides, line ranges.
- Risk: draft loss, comments posted on shifted lines.

### J-GIT-042 Bisect-style archaeology using graph search, blame and file history

- Status: new.
- Steps: 1. Search graph for message text, author, then sha prefix. 2. Open file history of renamed file (follows
  rename). 3. Blame at old commit; jump to parent of line's commit 5 times. 4. Copy sha, copy patch, copy file
  path at each step. 5. Open commit in host (browser).
- Oracle: clipboard after each copy: full 40-char sha, patch that `git apply --check` accepts on parent, repo-
  relative path; `Browser.Opened()` URL contains sha and host from remote config; history list equals
  `git log --follow --format=%H -- <path>`.
- Risk: rename not followed, short sha copied, wrong remote for host URL.
