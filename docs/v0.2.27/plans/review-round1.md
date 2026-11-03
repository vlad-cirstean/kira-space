# P470 review round 1 findings

Base: `17ad4b0` (v0.2.26 merge on `main`, `git merge-base main HEAD`). No earlier review commit on
this branch; chapters squash-merge, so prior review commits are not reachable. Diff: `17ad4b0..96e7438`.

Scope: P459–P469. Owned files plus one-hop callers: githost line comments (GitHub, Azure, gitsession,
gitrpc, review window frontend), review stamp poll, stage move `closeSessions`, repo-less task folder,
nanoid worktree folder and off-task `request_branches`, Jira detail cache, issue view markdown and
attachments, capture popup, panel tab store.

Counts: 1 High, 1 Medium, 2 Low.

## F1 (High) Alert titles inject raw HTML into host-view markdown

- File: `apps/kira-space/frontend/src/hostview/markdown.ts:57` (`md.use(alerts, …)`); renderer in
  `markdown-it-github-alerts@1.0.1` `alert_open`: `<p class="markdown-alert-title">${icon}${title}</p>`.
- The plugin takes a custom title from the rest of the marker line (`match[2]`) and emits it
  unescaped. `html_inline`/`html_block` stripping never sees it: the plugin rewrites the token in a
  core rule, and its own renderer writes the raw text.
- Verified: `> [!NOTE] <img src=x onerror=alert(1)>` renders `…</svg><img src=x onerror=alert(1)></p>`.
- Scenario: a GitHub PR body or comment (ChangeRequestView, HostViewConversation) or an Azure PR
  description holds that blockquote. `MarkdownBody.vue` mounts it with `v-html`. CSP `script-src
  'self'` blocks the inline handler, but any other markup lands: `<form action="https://…">` with
  inputs (`form-action` is not covered by `default-src`), `<style>` (allowed by `'unsafe-inline'`)
  restyling the whole window, `<meta http-equiv=refresh>`, links outside the chip and `SAFE_LINK`
  rules. This breaks the file's stated invariant "Raw HTML is never emitted". Jira text is not
  affected (`escapeText` escapes `!` and `<`), GitHub and Azure markdown are.
- Fix: after `md.use(alerts, …)`, override `md.renderer.rules.alert_open` to escape the title:
  `` `<div class="markdown-alert markdown-alert-${type}"><p class="markdown-alert-title">${icon}${md.utils.escapeHtml(title)}</p>` ``
  (`type` is one of the fixed markers, `icon` is the plugin's constant SVG). Add the payload above to
  the markdown render spec.

## F2 (Medium) A detail read in flight across a comment post re-saves the pre-comment copy

- Files: `apps/kira-space/internal/issuetracker/jira/detail.go:70-77` (`sf.DoChan` read then
  `saveDetail`), `detail.go:108-116` (`saveDetail` also calls `c.store`),
  `apps/kira-space/internal/issuetracker/jira/comment.go:51-55` (`forget` + `forgetDetail`).
- Scenario: the issue window opens with a stale saved copy, so `useIssueDetail` starts a fresh read
  (acli, seconds; up to `detailBound` 25 s). The user posts a comment meanwhile. `postComment` drops
  both caches, then the in-flight read finishes and `saveDetail` writes the copy read before the post
  into `details` and the Issue result cache. The composer's settled invalidation refetches with
  `fresh=false`: either it joins the same singleflight call, or it hits the re-saved copy, which is
  younger than `detailFreshFor` and returns as not stale. The new comment vanishes from the view
  (optimistic row replaced) for up to 30 s, and shows only after a manual Refresh. Same for the board
  Issue cache (`resultOKTTL`).
- Fix: a per-key generation in `client` bumped by `forgetDetail` (and `forget`); the read captures it
  before `readDetail` and `saveDetail`/`store` skip when it moved. After a successful post, also
  `c.sf.Forget("detail|"+key)` so the next read starts a new call instead of joining the old one.

## F3 (Low) A failed revalidation replaces the saved copy with an error

- Files: `apps/kira-space/frontend/src/hostview/queries.ts:44-50` (watch on `stale` calls
  `refresh()`), `apps/kira-space/internal/bridge/hostview.go:322-336` (an error returns
  `Kind: unavailable` with no detail).
- Scenario: offline or acli signed out, the user opens an issue viewed 5 minutes ago. The first call
  answers the saved copy marked stale ("Saved copy from … Updating."). The watch fires a fresh read,
  it fails, and the query data becomes the failure result: the readable copy disappears behind an
  error screen. The stale-while-revalidate design (P460 §2.2, §3.5) loses its main benefit exactly
  when the tracker is unreachable.
- Fix: in `useIssueDetail`, keep the previous `ok` result when a `fresh` read returns a non-ok kind
  (return the prior `query.data` from `queryFn`, stale flag kept), and surface the failure reason in
  the stale note instead of the full error state.

## F4 (Low) PR line comment target is a snapshot taken when the compose opens

- Files: `apps/kira-space/frontend/src/views/repo/reviewDecorations.ts:511`
  (`host: source.hostComment?.() ?? null`), `apps/kira-space/frontend/src/views/repo/ReviewThread.vue:57-59`
  (`hostTab` reads `props.host`).
- `createApp` gets a plain props object, so `host` is computed once. Scenario: the user opens a diff
  tab and starts a comment before the header's `changeRequest.reviewState` query answers (cold
  window, slow host): the PR tab is missing for that compose and does not appear when the state
  lands. The reverse also holds: a refocus refetch that finds the PR head moved does not set
  `blocked`, so Send stays enabled until the server answers `headMoved`.
- Fix: pass the getter (`hostComment: source.hostComment`) and make `hostTab` call it in the
  computed; `useHostLineComment` already returns a reactive getter over the query.

## Dimensions with nothing real

- Credentials: new `gh api` and `az` calls pass no token and read no CLI config. GitHub line comments
  send user text via `-f` only; `-F` carries integers Kira formats. No arg injection found.
- Paths: `ReviewWorkingStamp` validates each path with `ValidateRelPath`; task folder and nanoid
  worktree paths are server-built (`safeSegment`, `shortid`). No traversal found.
- Stage move: `closeStageSessionsLocked` runs under the task mutex like `CloseSession`; frontend and
  Go `stageSessions` rules match (`reviewStepID == "review"`). No race found.
- Panel tab store pruning, review stepping, repo-less runs on `taskTarget`, archive cleanup of the task
  folder: no defect found.
- Performance: stamp poll is one DB-backed branch load plus up to 5000 `lstat` per 1.5 s; issue
  prefetch runs only on a newly opened window and joins the window's own read through singleflight.
  No N+1 CLI calls and no listener leaks found.
