# v0.3.0 parallel batches (P538 to P548)

User asked for phase batches (CLAUDE.md loop rule exception). Base `b0941c84` (renumber commit). Up
to 3 whole phases per batch; each phase is its own stream: own worktree off the batch base, own Opus
plan, own Sonnet implementer. Discovery: `codegraph_explore` over the details panel
(`AdeDetailsTab`, `AdeTaskTimeline`, `taskTimeline`, `AdeHandoverList`, `AdeNotesEditor`,
`notesExtensions`), handovers (`ade/handover.go`, `repos/adehandover.go`), proof (`proof/service.go`,
`dbmcp/proof_tools.go`, `studiostorage/repos/proof.go`, llmprompts proof entries), workflow steps
(`adeflow/parse.go`, `adeflow/options.go`, `adewire.WorkflowStep`, `AdeStepInspector`), agent
sessions; shell greps for migrations, `StudioContract` callers and test dirs.

Result: 6 batches instead of 11 serial phases. P542 sits in B2 as a third stream (plan `P542.md`).

| Batch | Phases | Lands before |
|---|---|---|
| B1 | P538 | B2 starts |
| B2 | P539, P541, P542 | B3 starts |
| B3 | P540, P543 | B4 starts |
| B4 | P548 | B5 starts |
| B5 | P544, P545 | B6 starts |
| B6 | P546, P547 | chapter-end suite run |

## Rules every batch follows

- Gate: a batch starts (Opus plans included) only after the previous batch fully landed: every
  stream rebased onto `chapter/v0.3.0`, pushed, worktrees removed.
- Landing: table order inside the batch (lower `P` first); the next stream rebases on top.
  Resolve conflicts only in the shared files listed for that batch; any other conflict stops the
  landing (ownership boundary wrong).
- Migration dirs: each batch names one owner per dir (`internal/storage/migrations`,
  `internal/studiostorage/migrations`). A non-owner that finds it needs a SQL migration in that dir
  stops and reports; the orchestrator moves that phase to the next batch.
- Bindings tree (`frontend/bindings`) and i18n catalogs: additive per stream; regenerate bindings
  after each rebase, never hand-merge them.
- Each phase still meets its own `SPEC.md` row in full, acceptance checklist included.

## Order deviation

Table order is dependency order, with two batch exceptions the user asked for. B2 runs P541 and P542 before
P540 (B3); B4 runs P548 before P544 to P547. P542 depends on neither: it reshapes notes and handovers in the Space
DB and the details panel, and P540 moves YAML folders. P548 depends on none of P544 to P547.
SPEC sequencing says P541 to P543 share the details panel and run one at a time: P541 touches only the proof
window (`frontend/src/studio/proof/`), not the panel, so P541 beside P542 holds; P542, P543, P545, P546 stay serial.

## Pairs that cannot run together

- P538 with any later phase it would otherwise pair with. P538 rewrites the shared test harness:
  54 `StudioContract` callers (dbflow, dbmcpflow, proofflow, apiflow, dockerflow, termflow, others),
  `tests/ui/studio` fixtures and mock runtime, Playwright projects. P539 also edits
  `flowharness/contract.go` and the `tests/ui/studio/automations-*` specs P538 repoints. P541 edits
  `proofflow` tests and proof UI specs (both on the Studio harness today). P548 adds `dbmcpflow`
  tests. A new test written against `StudioContract` breaks once P538 lands. So B1 is P538 alone.
- P539, P540: strict chain, never two at once.
- P541 then P543: the chain orders diagram proof first; the diagram view comes from P541.
- P542 then P543: both rework handover display (P542 replaces `AdeHandoverList` with `AdeNotesChain`). P542
  lands first (B2); P543 plans against the unified chain and builds on it.
- P545 needs P542 and P543 (merged list of their entries).
- P546 and P547 need P545 (merged list and its edit surfaces). P546 and P545 also restructure the
  same list.
- P544 with P541, P542 or P543: each adds proof or handover schema that P544 folds; P544 runs after all three. P547 runs after P544 and stores its audit trail in the merged DB.
- P548 with B3 (P543) or B5 (P544): P548 runs after P543 (below); P544 deletes the Studio build stage
  where the DB MCP server is wired, which P548's per-session access changes. So B4 stays P548 alone.
- P539 and P548: both reshape the DB MCP token, helper script and agent registrations
  (`bridge/dbmcp.go`, `internal/mcpauth`, `internal/harness/claude/mcpinstall`).
- P543 and P548: both may add per-session identity to `dbmcp` (proof to task link; session
  credential). P548 runs after P543 and builds on what it added; P547 runs after both and uses it for
  session and task attribution.

## B1: P538

- Why alone: see first bullet above.
- Shared files: none (one stream).

## B2: P539 (home fold), P541 (Mermaid proof), P542 (unified notes)

- Why independent: P539 moves paths, the home, the second runs stack, the config-files twin, the DB
  MCP token. P541 adds a diagram view to proof steps, its tool schema and renderer. P542 moves task notes into
  the handover chain (Space `ade_task_handovers`, `internal/ade`, `bridge/adetask.go`, `capture.go`, notes
  components, stage dialog, add popover, handover tool text). The P539 fold moves both DB files whole, so a new
  Studio or Space migration applies wherever the file sits. None reads data another changes; P542 touches no
  home path (`flowharness.App.KiraHome`, which P539 deletes, stays unused).
- Migration owner: `studiostorage/migrations` P541 (rebuild of `proof_steps` and `proof_view_state`
  for the new view value, same pattern as `0048_p426_proof_chart_view.sql`, 0065); `storage/migrations`
  P542 (0077: `ade_task_handovers` rebuild with state `note`, notes copy, `ade_tasks.notes` drop). P539 needs
  neither (its plan); if it adds a Space migration anyway, P542 renumbers at its rebase (lands last).

| Shared file | P539 edit | P541 edit | P542 edit |
|---|---|---|---|
| `internal/flows/proofflow/helpers_test.go` | line 51 `app.KiraHome` helper token path to the Space home | none; new helpers go in a new file (`diagram_test.go`) | none |
| `tests/ui/support/mockRuntime.ts` | drop `StudioConfigFiles*` entries | none; diagram fixtures in `tests/ui/studio/support/proof.ts` | add `adeTaskAddNote` (near 239) and its invalidation (near 875) |
| `tests/ui/support/ipcChannels.ts` | drop studio config-files keys (495-496) | none | add `adeTaskAddNote` (near 265) |
| `internal/appwire/studio_build.go` | runs stack, dirs, config files (lines 41-80, 173) | only if proof deps change (lines 122-129) | none |
| `internal/llmprompts/registry.go` | none | proof entries (near 1465) | handover notes arg (near 1003), `ade.handoverLine` (near 1283) |
| `internal/llmprompts/testdata/budget.tsv` | none | lines 228, 240, 241 | lines 109, 132 |
| i18n catalogs | backend keys if any | `studio.proof.diagram.*` | `space.ade.notes.*` |
| `docs/ARCHITECTURE.md` | home fold sections, Known open item | proof section | "Stage handover (P407)" section, Task panel line |

- Not shared: `dbmcp/server.go` (P539), `dbmcp/proof_tools.go` (P541), `package.json`/`bun.lock` (P541,
  Mermaid), `internal/ade/**`, `bridge/adetask*.go`, `bridge/capture.go`, `adewire/wire.go`,
  `frontend/src/ade/v2/**`, `tests/contract/ade-*.json` (P542).
- Landing: P539, then P541, then P542.

## B3: P540 (config subfolder), P543 (proof chain)

- Why independent: P540 moves YAML folders and adds one config-root resolver (`configfs`,
  `connfiles`, `apifiles`, `adeflow` dirs, `repocfg`, `agentcfg`, appwire joins). P543 links proof
  to tasks and shows proof, links and handovers as a chain in the details panel. Neither reads the
  other's data.
- Migration owner: `studiostorage/migrations` P543 (proof to task link); `storage/migrations` P543
  if its link lives in the Space DB. P540's layout move is a filesystem migration, not SQL.

| Shared file | P540 edit | P543 edit |
|---|---|---|
| `internal/appwire/appwire.go` | config folder map and home joins (around lines 543, 566, 795) | proof wiring into the task board deps, own hunk |
| `internal/appwire/studio_build.go` | `connections`, `api` dirs (lines 60, 80, 168, 173) | `proofDeps` (lines 122-129) |
| `docs/ARCHITECTURE.md` | config subfolder section | proof chain section |

- Not shared: `internal/llmprompts/registry.go` (P543, terse-handover rule), `frontend/src/ade/v2/**` (P543), `dbmcp/proof_tools.go` (P543).
- Landing: P540, then P543.

## B4: P548 (Studio MCP option in steps)

- Why alone: P548 runs after P543 (B3) and cannot pair with P539 (B2) or P544 (B5); see pairs above.
- Migration owner: both dirs P548, if its plan needs a table (default: session credential in memory).
- Shared files: none (one stream).

## B5: P544 (DB merge), P545 (timeline toggle)

- Why independent: P544 is backend storage, wiring and the Studio settings and op-log frontend
  (`studio/state/*`, `SettingsDialog`, `OperationsDock`, `createOpLogStore`). P545 is the details
  list only (`AdeTaskTimeline.vue`, `taskTimeline.ts`, the chain component from P542/P543 (`AdeNotesChain.vue` or its P543 successor),
  `state/adePanelTabs.ts`). Condition: P545 keeps toggle persistence out of the settings table (a
  Pinia store, as `adePanelTabs` does today); a plan that needs a settings key moves P545 to B6.
- Migration owner: both dirs P544. P545 adds none.

| Shared file | P544 edit | P545 edit |
|---|---|---|
| `tests/ui/support/mockRuntime.ts` | drop `StudioSettings*`, `StudioOps*` entries | none expected; timeline fixtures in its spec |
| i18n catalogs | drop Studio settings and op-log keys | timeline toggle keys |

- Landing: P544, then P545.

## B6: P546 (forward panel edits), P547 (Kira DB audit trail)

- Why independent: P546 hooks the panel's edit paths (notes, links, proof) and delivers a diff to
  the step's running agents (`internal/ade`, `bridge/adetask.go`, agent sessions). P547 captures
  every Kira DB call and approval in `dbmcp`, stores it in the merged DB and adds an entry kind to
  the merged list. P547's prerequisites: v0.2.36 P528 and P537 (landed), P544 (merged DB), P545
  (merged list), P548 (session attribution); all landed by B5.
- Migration owner: `storage/migrations` P547. P546 adds none.

| Shared file | P546 edit | P547 edit |
|---|---|---|
| merged list item component (from P545) | forward offer on editable entries | audit entry row, new kind |
| merged list model (`taskTimeline.ts` or its P545 successor) | none expected | audit entry kind and source |
| `internal/appwire/appwire.go` | register forward service if new | register audit service |
| `frontend/src/ade/v2/queries.ts`, `wire.ts` | forward mutation and types | audit query and types |
| i18n catalogs | forward keys | audit keys |

- Landing: P546, then P547.
