# v0.3.22 code review, round 1

Base: `03d33b5ca` (v0.3.21 squash merge on `main`; holds v0.3.21's review fixes, no later review
session exists). Diff `03d33b5ca..0315ecb7a`, phases P707-P715. Reviewer: Opus.

Scope read: installer and release index (`scripts/install.sh`, `internal/releaseline`,
`internal/appupdate`, `scripts/releaseindex`, `release.yml`, `release-retention.yml`,
`verify-packaging.sh`), claude hook dirs (`internal/harness/**`, `ade/tracker.go`), state schema
(`internal/agentmcp`), shortcuts (`shortcuts.ts`, `moduleShortcuts.ts`, `moduleActions.ts`,
`shortcutText.ts`, `keys.ts`, Go menus, `App.vue`), approvals (`studio/approvals/*`,
`workbench/approvals/*`), dialog primitives, button variants, N17/N18/N19 guards, ADE plan model.

Standing rules checked, nothing found: every new or touched `.vue` is `<script setup lang="ts">`,
no new scoped `<style>`; new state in Pinia (`newScriptRequest`), server state through TanStack
Query, VueUse for resize/scroll/listeners. Button tint removal: no resting fill left on any
`dialog*` variant or class override (`bg-*/15` hits are badges, alerts, menu focus, log rows).
CLI credentials (G5): untouched; only CI uses `gh` with the workflow token. Test bar: new unit
tests guard a strict parser (`index_test.go`), the resume schema subset, and the N18/N19 guards;
nothing needless. Dead code: none beyond the stale comments below. P708 hook dirs, P713 schema
change, app update checker: no finding.

## Findings

### 1. important: cell popover rewrites the value it shows and copies

`apps/kira-space/frontend/src/studio/approvals/PendingResponse.vue:191-193` (`showCell`).

A cell that parses as a JSON object or array is shown as `JSON.stringify(JSON.parse(cell), null, 2)`.
`JSON.parse` turns every number into a float64, so `{"id": 9007199254740993}` displays as
`9007199254740992`, `1.10` as `1.1`, `1e2` as `100`. The popover's Copy (line ~408) copies that
rewritten text, and the char count (line 405) counts it. Failure: an approver reviewing a held
result opens a jsonb cell holding a bigint id, copies it, and gets a different id than the bytes
the agent receives. The component's own header says nothing here re-renders the held bytes.

Fix: show and copy `cell` unchanged. If pretty JSON is wanted, colourise/format with a
lossless formatter (indent by token, no number round-trip; Monaco `colorize` via
`OpLogCodeBlock language="json"` over the raw text) and keep Copy on the raw cell.

### 2. nit: one tooltip button and one resize handle per visible cell

`PendingResponse.vue:355-379`. Every rendered cell mounts a `TooltipIconButton` (Tooltip root,
trigger, Button) and a `ResizeHandle`. The table has no column virtualization (`w-auto`), so a
40-column result with ~28 rendered rows (12 visible plus overscan 8 each side) mounts about 1,100
of each, recreated on every scroll page. Only the first cell's handle is tabbable; the rest are
duplicates. Fix: one handle per row (first cell, or a row-level overlay), and one shared expand
button positioned on the hovered cell (same pattern as the one shared popover).

### 3. nit: awk stepping ignores the index's asset column

`scripts/install.sh:75` (`index_max`) and `:98` (`index_next_step`). Both rank every line's tag
without checking `$2 == $ASSET`; only `index_entry` filters. Go `NextStep` filters through
`Eligible(r, asset)`. Index format 1 allows any asset name per line, so an index that ever lists a
second asset (Intel build, second platform) makes the awk pick a tag that has no `$ASSET` line and
die "not a published release", while the app picks correctly. S16's golden has one asset, so it
cannot see the drift. Fix: skip lines whose `$2 != a` in both functions (pass `-v a="$ASSET"`), or
make `index_check` require every line's asset to be `$ASSET`.

### 4. nit: release index lists releases retention later deletes

`.github/workflows/release.yml` (Build and upload the release index) with
`release-retention.yml`. The index is built once at publish; retention runs after and deletes
non-target releases whole. Stepping targets are always kept, so the updater is safe, but
`install.sh --tag=<deleted>` passes the index lookup and dies at download with "disk image is not
downloadable (HTTP 404)" (`install.sh:294`) instead of "not a published release" (`:277`).
Fix: either reword the 404 message to cover a removed release, or have retention re-upload the
latest release's index after deletions (`gh release upload --clobber`).

### 5. nit: Dialog X never shows its chord tooltip to keyboard users

`packages/theme/src/components/ui/dialog/DialogHeader.vue:31`. The capture listener stops every
`focus` event to the X, including Tab focus, so the Esc / Shift+Esc chip (P712) is reachable by
hover only. The P712 manual checklist expects tooltips on Tab focus. The root cause is the
auto-focus on open eating the first Esc. Fix: ignore only the programmatic open focus (for example
skip while `:focus-visible` is false, or let the tooltip close without consuming Esc), not every
focus.

### 6. nit: orphaned doc comment

`packages/theme/src/components/ui/dialog/discard.ts:16-18`. `DIALOG_PROMPT_KEY` was inserted
between `DIALOG_REQUEST_CLOSE_KEY` and its doc comment, so two doc comments stack on
`DIALOG_PROMPT_KEY` and the close key has none. Fix: move line 16 back above
`DIALOG_REQUEST_CLOSE_KEY`.

### 7. nit: stale comments

- `apps/kira-space/frontend/src/studio/views/shared/request/useRequestTabSave.ts:22-24` names
  `registerCommand('api.save', onSave)` and "the command palette entry it registers". P711 removed
  the `api.save` palette entry and no view registers `api.save`. Fix: drop the clause.
- `apps/kira-space/frontend/src/views/repo/ReviewThread.vue:5-10`: the file header comment now sits
  between imports (import sort moved three imports above it). Fix: move it above line 2.

### 8. nit: URL userinfo tokenized as port

`apps/kira-space/frontend/src/studio/approvals/requestLine.ts:43-47` (`portAt`). For
`https://user:pass@host/x` the authority is `user:pass@host`; the last `:` is inside the userinfo,
so the request line renders host `user` and port `:pass@host` (port style). Fix: search for the
port only after the last `@` of the authority.

## Dimensions

- Architecture, structure, security: findings 3, 5, 6, 7. Release index trust root is unchanged
  (hash from the same release as the binary); `KIRA_INSTALL_SOURCE_ONLY` seam is inert in a real
  run. No security finding.
- Functional correctness: findings 1, 4, 8.
- Performance: finding 2. Release index replaces a paginated API listing with one GET; nothing else.

Counts: 1 important, 7 nit.
