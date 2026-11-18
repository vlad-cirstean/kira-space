# P672 Part 10 batch: iterations 7, 8, 9

Streams, each own worktree off `chapter/v0.3.17` tip. User asked for 3 at once. Subjects differ (status/tones, tabs/segmented/choice, states/icons/type/spacing); overlap is additive.

| Stream | Iteration | Plan | Branch |
|---|---|---|---|
| D | 7 status/tones | `P672-part7-iter2.md` | `stream/v0.3.17-p672-i7` |
| E | 8 tabs/segmented/choice | `P672-part8-iter2.md` | `stream/v0.3.17-p672-i8` |
| F | 9 states/icons/type/spacing | `P672-part9-iter2.md` | `stream/v0.3.17-p672-i9` |

## Guard ids (fixed here)

- D: G7a-G7e become U147-U151.
- E: U117 extended in place; G8a becomes U152.
- F: U73, U88, U90 extended in place; G9a-G9d become U153-U156.

## Shared files (additive hunks)

- `scripts/check-ui-primitives.sh`: each stream appends its guards in its own id range; in-place extensions edit only the named line.
- `docs/ARCHITECTURE.md`, `docs/v0.3.17/SPEC.md`: docs commits last per stream; SPEC edit is the Part 10 progress line only. Guard-range numbers in ARCHITECTURE end at U156 after all land: each stream writes its own max, landing resolves to U156.
- Theme: D owns `statusTone.ts`, `connColor.ts`, Button variants index, `TooltipIconButton.vue`; E owns toggle `index.ts`, `SecondaryTabs.vue`; F owns `Disclosure.vue`, `frames.ts`.

## Cross-stream rulings

- Http/GrpcRequestView unsaved `•` glyph to warn StatusDot: D owns (plan D F13). F skips those two files in its F2.
- git-ui current-branch marker (WorktreeList, StackList, RefsPopoverBranches) to muted StatusDot: F owns. D does not touch them.
- AdeAuditEntry Spinner swap: D. docker ContainerDetail/EngineOverview action icons: D drops tone classes; F touches icon names only if needed, never classes D removes.
- AdeNotesEditor: E owns the format toggles; F skips its glyph lines unless disjoint.
- MemoryPanel/MemoryContext/VariablesOverviewPanel: F owns opacity changes; D F9 touches memory import files only.
- ContainerEditView: E changes the tab dirty dot tone only if D has not; D owns tone.
- Out of scope for E: the plan's "decision list" NativeSelects. Carried to the final scan.

## Landing

Order D, E, F. Rebase each onto chapter tip (`--onto`), resolve only listed shared files, stop on any other conflict. Duplicate-line merges in the guard script (an in-place edited line kept next to its old version) must be de-duplicated. Re-run `scripts/check-ui-primitives.sh`, `bun run lint:dead` and typecheck after each rebase, before push. Targeted specs per plan; implementers use `--workers=2`.
