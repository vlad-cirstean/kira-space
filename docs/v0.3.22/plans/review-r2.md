# v0.3.22 code review round 2

Base: `03d33b5ca` (v0.3.21 squash merge on main), same as round 1. Diff `03d33b5ca..a572d0d1e`.
Focus: round 1 fixes `292750755`, `4b5d417a0`, `3ac04dda4`, `acfddfff5`, `7ccc5e24e`, `a572d0d1e`.

Checked, nothing found: install.sh `index_max`/`index_entry`/`index_next_step` asset filter (one line
per tag in both `index_check` and Go `ParseIndex`; Go `Eligible` filters by asset too, so parity
holds), 404 wording (matches ARCHITECTURE and PACKAGING), `check-install-step.ts` `ASSET=app.dmg`
(golden holds only `app.dmg`), `portAt` userinfo (`user:{{p}}@host`, `u:p@host:8080`, `[::1]:8080`
all split right), raw-cell popover (shows and copies `cell` unchanged), comment fixes in
`discard.ts`, `ReviewThread.vue`, `useRequestTabSave.ts`.

Counts: 3 important, 1 nit.

## 1. Dialog X: Tab flag never clears, open focus after any earlier Tab shows the tooltip (important)

`packages/theme/src/components/ui/dialog/DialogHeader.vue:32-48`. `tabbing` is set on every
keydown and only reset by the next keydown. Pointer input and async opens leave it `true`.

Failure: user presses Tab (form field, Monaco indent, list), then mouse-clicks a button that opens a
dialog, or an approval prompt opens on its own. Dialog open focus lands on the X with
`tabbing === true`, focus is not swallowed, tooltip opens, first Esc closes the tooltip instead of
the dialog. Exactly the P712 bug `358370bb8` fixed, back for any session that used Tab once. The new
spec only covers open-by-click with no prior Tab.

Fix: scope the flag to the Tab's own focus move. In the keydown handler set `tabbing = true` for Tab
and clear it after the default action (`setTimeout(() => { tabbing = false })`), or also clear it on
`pointerdown` (capture) and after the focus handler reads it. Add the Tab-then-click case to
`shortcut-hints.spec.ts`.

## 2. Data review: keyboard reaches the expand button of the first filled column only (important)

`apps/kira-space/frontend/src/studio/approvals/PendingResponse.vue:195-205, 390-400`. Focus in a row
sets `activeCell` to `firstFilledCol(r)`. The expand button is the only focusable element in a data
cell, so focus can never enter another `td[data-col]` and `trackCell` never moves the column.

Failure: held row `[id, name, notes]`, long text in `notes`. Keyboard user tabs row, chevron,
handle, `id`'s expand, then the next row. `notes`' popover (full value, Copy) is unreachable without
a mouse. Before `acfddfff5` every non-null cell had a button in the Tab order.

Fix: roving column on the mounted button: ArrowLeft/ArrowRight on `db-mcp-preview-expand` move
`activeCell.col` to the previous/next non-null column and focus the new button after `nextTick`.
Spec: Tab to the expand button, ArrowRight, Enter opens the second column's value.

## 3. Data review: pointer and focus share one `activeCell`, hover unmounts the focused button (important)

`apps/kira-space/frontend/src/studio/approvals/PendingResponse.vue:201-214`. `trackCell` on
`mouseover` overwrites the cell keyboard focus mounted the button in, and it ignores `cellOpen`
(only `releaseCell` checks it).

Failure A: keyboard focus on row 3's expand button; pointer rests over row 7 or another cell of
row 3. `mouseover` moves `activeCell`, the focused button unmounts, focus drops to `body`.
Failure B: popover open from Enter on the expand button; pointer moves over the table. The button
that opened the popover unmounts, so Escape's focus return has no target and focus lands on `body`.
Failure C: popover closed after the pointer left the row: `releaseCell` skipped while open, nothing
re-runs on close, button stays mounted on a row nobody hovers.

Fix: two refs, `hoverCell` (mouseover/mouseleave) and `focusCell` (focusin/focusout); mount the
button where either matches. Skip `trackCell` while `cellOpen`, and on popover close re-evaluate
hover (clear `hoverCell` when the anchor row is no longer `:hover`).

## 4. `trackCell` writes a fresh object on every `mouseover` (nit)

`apps/kira-space/frontend/src/studio/approvals/PendingResponse.vue:203`. `mouseover` fires at each
child boundary (td, span, button); each assignment is a new object, so the whole table render
(visible plus 8 overscan rows times columns) re-runs even when row and column are unchanged.

Fix: return early when `activeCell.value?.row === r && activeCell.value.col === col`.
