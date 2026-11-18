# v0.3.17

UI uniformity across all modules. Continues from chapter v0.3.16 (P672 and P660 Part 2 moved here, keep their numbers).
Phases from `P672` (highest shipped: `P671`, chapter v0.3.16). Next new phase: `P673`. Branch
`chapter/v0.3.17`.

| # | Phase | Status |
|---|-------|--------|
| P672 Part 1 | UI inventory and uniformity rules | Done |
| P672 Part 2 | UI uniformity: theme pieces and guards | Planned |
| P672 Part 3 | UI uniformity: forms and labels | Planned |
| P672 Part 4 | UI uniformity: buttons and dialogs | Planned |
| P672 Part 5 | UI uniformity: menus, popovers, tooltips | Planned |
| P672 Part 6 | UI uniformity: headings, detail headers, key-value, cards | Planned |
| P672 Part 7 | UI uniformity: status, tones, colours | Planned |
| P672 Part 8 | UI uniformity: tabs, segmented, choice | Planned |
| P672 Part 9 | UI uniformity: states, icons, type, spacing | Planned |
| P660 Part 2 | Git review panel state across module switches | Planned |
| P672 Part 10 | Second pass over Parts 2..9 (iter2 plans) | Planned |

## P672 Part 1: UI inventory and uniformity rules

User feedback: nothing in the UI is uniform. Not only per module: each dialog, menu, card in the same module differs in components and design. "Elements" means more than dialog and card: how information shows inside them. Covers alignment, colors, fonts, sizes, spacing, labels, icons, empty/loading/error states, headers/footers, button placement and order, form layouts, lists/tables/rows, tooltips, badges. Part 1 is analysis only, no UI change. Deliver: (1) exhaustive inventory of element types and each variant actually used, with counts and file references (via codegraph), grouped per element type; (2) a small set of canonical patterns; (3) a mapping of every variant to its canonical pattern, or a justified exception. Output committed under `plans/`: inventory, canonical rules, migration list. From the inventory the planner decides how to cut Part 2 into Parts 2, 3, ... by element type or module, each sized for one planner and one implementer; the orchestrator then adds them as rows. Acceptance: exhaustive (every dialog, menu, card, list, form in every module appears) and canonical rules concrete enough to review (token values, component to use, markup shape). Tests: none (docs).

Plan: `plans/P672.md`.

Result: Done. Inventory `plans/P672-inventory.md` (716 `.vue` files, 14 modules; codegraph walks plus template and class-token scans at `6527aec4`). Finding: primitives are uniform and lint-held; divergence sits in composition (primitive picked per role, module-local wrappers, free spacing/type, two colour vocabularies). 16 families with canonical rules (RD dialogs to RTa tabs), ranked top 12 inconsistencies (labels in two colours and five layouts; running = info in Runs vs warn in ADE and ADE pastels leaking into Runs; 11 hand detail headers; three button looks for in-view actions; 24 popover class variants), migration items M1-M36, appendix A.1-A.13 listing every dialog, popover, card, heading, menu, list, form and tabs host. Apply work cut into Parts 2..9 (plan `plans/P672.md`): Part 2 theme pieces and guards first, then sequential; optional batch pairs {5, 6} and {7, 8}. Decisions for the user: D1 ADE palette scope (blocks Part 7), D2 new shared pieces (`DetailHeader`, `Field orientation="responsive"`, `PopoverContent flush`, `KeyValueList density`, `DetailSection`/`TextTooltip` in theme), D3 toast scope (blocks Part 9 toast item). No app code changed; no tests (docs).

## P672 Part 2: UI uniformity: theme pieces and guards

New shared theme pieces and lint guards every later Part builds on (`plans/P672-inventory.md` §20 M1-M3). Theme: `Field orientation="responsive"` (label column 7.5rem at `@24rem`, stacked below), `PopoverContent flush`, `KeyValueList density="compact"`, `DetailHeader` composite (icon 16, title lg 500, meta and actions slots), Docker `DetailSection` and ADE `AdeTip` moved into theme (`TextTooltip`), imports switched. Guards in `scripts/check-ui-primitives.sh` for the regex-checkable rules (checkbox restyle, popover padding, heading classes, KV grid overrides, `font-mono`, toned menu icons, segmented in forms, ADE tones off-board after D1), each with a dated temporary baseline of current offenders, never PERMANENT. ARCHITECTURE "UI architecture" gains the canonical rules RD..RTa in short form. No call-site restyle beyond the two moves. Tests: affected UI specs run; no new unit tests.

Plan: `plans/P672-part2.md`.

## P672 Part 3: UI uniformity: forms and labels

Apply RF.1-RF.8 (`P672-inventory.md` §4, M4-M9) in every module: one field label (`FieldLabel`), stacked or `Field orientation="responsive"` columns (retire `AdeInspectorRow`, `EditField` label, `AdeRow` editable label column), checkbox rows as `Field horizontal` (drop `CheckboxRow`, checkbox `size-*` and custom indicators, 34 uses), `FieldSet` without gap overrides, width and textarea steps, hand error text to `FieldError`/`InlineState`/`Alert`, `kira` inputs in forms to default. Files: appendix A.4, A.6, A.9, A.13. Empties this Part's guard baselines. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part3.md`.

## P672 Part 4: UI uniformity: buttons and dialogs

Apply RB.1-RB.3 and RD.1-RD.6 (§2, §3, M10-M15): button size by host, outline only to commit or decide, in-view actions `toolbar@kira`, ADE `ACTION_CLASS` board-only; dialog sizes per table, one emphasised footer button rightmost (5 dialogs), header title only, intro as `DialogDescription`, no `DialogBody` overrides. Files: §3.1 hosts, A.1. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part4.md`.

## P672 Part 5: UI uniformity: menus, popovers, tooltips

Apply RM.1-RM.4 and RT.1-RT.2 (§5, §11, M16-M19): hand `DropdownMenuItem` lists (21 files) to `MenuItems`, menu icons muted 13 untoned, every `PopoverContent` (41) to default form shape or `flush` list shape with section rows, widths w-56/80/96/120, plain text tooltips via `TextTooltip`. Files: A.2, A.11. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part5.md`.

## P672 Part 6: UI uniformity: headings, detail headers, key-value, cards

Apply RH.1-RH.4 and RC.1-RC.3 (§7, §8, M20-M23): the 11 hand detail headers onto `DetailHeader`, the 13 `h3 text-kira-lg` section titles to `SectionHeading`/`FieldLegend`, `KeyValueList` only (two densities, no grid overrides, hand KV grids replaced), cards per job (inset, section, list), code blobs as `CodeBlock`, card-as-row to rows. Files: §7.1, A.3, A.5. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part6.md`.

## P672 Part 7: UI uniformity: status, tones, colours

Needs user decision D1 first (`P672-inventory.md` §19). Apply RS.1-RS.3, RL.2, RL.4, RI.2, RCo.1-RCo.2 (§6, §9, §12, M24-M27): one status-to-tone map (running = info, so `AdeOpStatus` leaves warn), ADE pastel tones per D1 (recommended: board and task cards only; Runs script rows and every other surface on theme tones and `Badge`), warning/error icon colours, row meta subtle, redundant `text-fg`. Files: A.7, `AdeChip`/`AdeOpStatus` hosts, Git change-request badges. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part7.md`.

## P672 Part 8: UI uniformity: tabs, segmented, choice

Apply RTa.1-RTa.2 (§17, M28-M29): `SecondaryTabs` variant by role (form value `choice`: 9 form `segmented` uses move; content `tabs` at default size; bar view switch `segmented@kira`), `ConnectionEnginePicker` onto `RadioGroupCard` (engine icon 16) and its U36 PERMANENT line deleted. Files: A.8. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part8.md`.

## P672 Part 9: UI uniformity: states, icons, type, spacing

Apply RE.1-RE.4 (toast item needs decision D3), RI.1, RTy.1-RTy.3, RSp.1-RSp.2, RBo.1-RBo.3 (§10, §13-§16, M30-M36): `EmptyState` shape and no toned media, three skeleton shapes, toasts only for off-screen outcomes, icon 13 in labelled buttons, size roles, `font-mono` to `font-data`, pane roots `p-3`, radii, borders, shadows. Deletes every remaining temporary guard baseline from Part 2. Files: A.10, class-token offenders by fresh scan. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part9.md`.

## P660 Part 2: Git review panel state across module switches

Git review segment (`RepoReviewView.vue` hosting git-ui `ReviewView`) loses its in-flight review, file filter and scroll on module switch. git-ui is its own Vue app with no Pinia (`packages/git-ui/src/main.ts` `mount`, `NullViewStateStore`). Fix changes git-ui's mount contract so it keeps view state across module switches. Part 1 passes `stateKey: null` for git-ui's `usePanelHeaderSearch` callers (`ReviewView.vue`, `FileTree.vue`). Acceptance: after switching away and back, review segment restores in-flight review, file filter and scroll; own plan `plans/P660-part2.md`, UI spec, same rules as Part 1.

Continues P660 from chapter v0.3.16 (Part 1 Done there); keeps its number.

Plan: `plans/P660-part2.md`.

## P672 Part 10: Second pass over Parts 2..9 (iter2 plans)

Standing user instruction: after all phases (P672 Parts 2..9 and P660 Part 2) are done, run a second full pass of the P672 UI uniformity Parts. User expects the first pass missed many things. Each of Parts 2..9 runs again, plan named `plans/P672-part<k>-iter2.md`, planned against the tree as the first pass left it. Planner re-reads current source and a fresh scan (codegraph, class-token), never trusts the first pass's summary or Results. Per working agreement "Multiple passes": one Opus plan and one Sonnet implementer per Part, in Part order. Part 10 planner either cuts this row into per-Part iter2 plans or runs it as one loop. Row names get no new numbers. Guard baselines from Part 2 stay empty. Chapter-end code review round follows this row (reviewer covers the whole chapter diff: findings, fix), then close. Tests: UI specs updated; visual baselines in PR CI; no new unit tests.

Plan: `plans/P672-part10.md`.
