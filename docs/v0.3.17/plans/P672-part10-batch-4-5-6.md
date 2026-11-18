# P672 Part 10 batch: iterations 4, 5, 6

Streams, each own worktree off `chapter/v0.3.17` tip. User asked for 3 at once. No ordering dependency: subjects differ (buttons/dialogs, menus/popovers/tooltips, headings/cards/KV); overlap is additive.

| Stream | Iteration | Plan | Branch |
|---|---|---|---|
| A | 4 buttons/dialogs | `P672-part4-iter2.md` | `stream/v0.3.17-p672-i4` |
| B | 5 menus/popovers/tooltips | `P672-part5-iter2.md` | `stream/v0.3.17-p672-i5` |
| C | 6 headings/cards/KV | `P672-part6-iter2.md` | `stream/v0.3.17-p672-i6` |

## Guard ids (fixed here, avoids collision)

- A: U134-U140 as planned, U118 extended.
- B: plan's G5a-G5e become U141-U145.
- C: plan's G6a becomes U146.

## Shared files (additive hunks)

- `scripts/check-ui-primitives.sh`: each stream appends its guards at the end of its own block; ids above.
- `docs/ARCHITECTURE.md`, `docs/v0.3.17/SPEC.md`: docs commits last in each stream; SPEC edit is the Part 10 progress line only. Conflicts resolved at landing.
- Theme: A owns `Button.vue`, `DialogFooter.vue`, `i18n/en.json`; B owns popover index, `TextTooltip`, `MenuItem`/`MenuItems`; C owns `SectionHeading.vue`, `ScriptProgress.vue`, `TitleBarTitle.vue`.

## Per-file overlap (touched by 2+ streams)

Plan "Files touched" lists are authoritative. Cross-stream files named by planners: ColumnsMenu, ProjectionMenu, MemoryFilters, RefsPopoverBranches, AdeLeasesPane, RepoFileEditBar, git-ui App.vue, EnvironmentsView (A+B); ImportJobDetail, WorktreeList, StackList, AdeLeasesPane, ContainerSizeSection, EngineDiskSection, AdeWorkflowReviewPanel (A+C); AdeRunDialog, AdeAgentDialog, ApiAgentAccess, FiltersDialog (A+C); CellEditorView, AdeLinkStrip (B+C, disjoint lines). Each stream edits only its own concern there.

## Handoff A to B

Stream B also converts: raw `kira-icon` Buttons to TooltipIconButton (AdeReviewRemoved, AdeRepoGateDialog, ApiAccessRulesSection, ConsoleResultStrip, EnvironmentsView ~160, FailureBanner); hand Kbd in TooltipIconButton (AdeTaskCard ~213, AdeBranchRow ~422/~556); hand Tooltip + Kbd (RefreshButton, RepoFileEditBar ~43). Stream A skips those.

## Landing

Order A, B, C. Rebase each onto chapter tip (`--onto`), resolve only listed shared files, stop on any other conflict. Re-run `scripts/check-ui-primitives.sh` after each rebase. Targeted specs per plan; implementers use `--workers=2`.
