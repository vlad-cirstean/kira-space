# P543 Result (draft)

User notes now rows in `ade_task_handovers` (state `note`); agent handovers and user notes form one chain, one Markdown editor everywhere.

- Migration 0077 rebuilds `ade_tasks` without `notes`, copies non-blank notes into note rows. `Task.notes` stays on the wire, derived read-only (user notes newest first, joined by blank line).
- New bridge method `AddNote`. Capture text lands as one user note. Limits: note 1 MiB, draft/handed 16 KiB.
- Frontend: `AdeNotesChain` replaces `AdeHandoverList`; `AdeNotesEditor` used by chain, stage dialog, Add popover. Composer draft in `adeNoteComposer` Pinia store.
- Prompts: handover notes arg and `ade.handoverLine` reworded for the merged chain.

## Verification
Pass: go build/vet, lint:go, lint, typecheck, lint:dead, build, TestUnifiedNotes, TestMigration77MovesNotesIntoChain, ade-v2-notes.spec.ts (6), Markdown unit spec (13), updated ade-v2-handover/panel/task-panel/archived/add and ui-leftovers specs (102 pass).
Fail, pre-existing on base dc9377dd, unrelated (databases http response tests in ui-leftovers.spec.ts: `http-send` never appears): "pane InlineState: response formatting..." and "response find bar...". Left out of scope, needs own look.
Real `claude` run: not run, no API key.

## Deviations
- Editor prop `focusOnMount`, not `autofocus` (biome noAutofocus).
- Hard breaks via `notesDoc.ts` `parseWithLineBreaks` (JSON transform), not marked options: marked options are global and leak across editors.
- Composer store has `clear`, no `hide`.
- `useUpdateHandover` also invalidates the handovers prefix.
- `budget.tsv` raised also for `ade.manual.message`, `ade.review.message`, `ade.start.message` (inlined handover line); 8 goldens regenerated.
- Extra files outside plan table: captureflow/capture_test.go, journeyflow/firstrun_test.go, adeflow/archived_test.go, contracts ade-review-convert/ade-task-statuses (only `"notes": null` removed), repos adetask_detach/restore tests, ade/board_test.go, llmprompts goldens, ui-leftovers.spec.ts, ade-v2-archived.spec.ts.
- Mobile step-move notes (`PlanTaskCard.vue`) stays plain Textarea. User to confirm.
