# v0.3.0 parallel batches (P538 to P548)

User asked for phase batches (CLAUDE.md loop rule exception). Base `b0941c84` (renumber commit). Up
to 3 whole phases per batch; each phase is its own stream: own worktree off the batch base, own Opus
plan, own Sonnet implementer. Discovery: `codegraph_explore` over the details panel
(`AdeDetailsTab`, `AdeTaskTimeline`, `taskTimeline`, `AdeHandoverList`, `AdeNotesEditor`,
`notesExtensions`), handovers (`ade/handover.go`, `repos/adehandover.go`), proof (`proof/service.go`,
`dbmcp/proof_tools.go`, `studiostorage/repos/proof.go`, llmprompts proof entries), workflow steps
(`adeflow/parse.go`, `adeflow/options.go`, `adewire.WorkflowStep`, `AdeStepInspector`), agent
sessions; shell greps for migrations, `StudioContract` callers and test dirs.

Result: 6 batches instead of 11 serial phases.

| Batch | Phases | Lands before |
|---|---|---|
| B1 | P538 | B2 starts |
| B2 | P539, P541 | B3 starts |
| B3 | P540, P542 | B4 starts |
| B4 | P543, P548 | B5 starts |
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

Table order is dependency order. B4 runs P548 before P544 to P547; the user-asked batch exception
covers that. P548 depends on none of them.

## Pairs that cannot run together

- P538 with any later phase it would otherwise pair with. P538 rewrites the shared test harness:
  54 `StudioContract` callers (dbflow, dbmcpflow, proofflow, apiflow, dockerflow, termflow, others),
  `tests/ui/studio` fixtures and mock runtime, Playwright projects. P539 also edits
  `flowharness/contract.go` and the `tests/ui/studio/automations-*` specs P538 repoints. P541 edits
  `proofflow` tests and proof UI specs (both on the Studio harness today). P548 adds `dbmcpflow`
  tests. A new test written against `StudioContract` breaks once P538 lands. So B1 is P538 alone.
- P539, P540: strict chain, never two at once.
- P541 then P542: the chain orders diagram proof first; the diagram view comes from P541.
- P542 and P543: both rework handover display (`AdeHandoverList`, handovers in a chain).
- P545 needs P542 and P543 (merged list of their entries).
- P546 and P547 need P545 (merged list and its edit surfaces). P546 and P545 also restructure the
  same list.
- P544 with P541, P542 or P543: each adds proof or handover schema that P544 folds; P544 runs after all three. P547 runs after P544 and stores its audit trail in the merged DB.
- P539 and P548: both reshape the DB MCP token, helper script and agent registrations
  (`bridge/dbmcp.go`, `internal/mcpauth`, `internal/harness/claude/mcpinstall`).
- P542 and P548: both may add per-session identity to `dbmcp` (proof to task link; session
  credential). P548 runs after P542 and builds on what it added; P547 runs after both and uses it for
  session and task attribution.

## B1: P538

- Why alone: see first bullet above.
- Shared files: none (one stream).

## B2: P539 (home fold), P541 (Mermaid proof)

- Why independent: P539 moves paths, the home, the second runs stack, the config-files twin, the DB
  MCP token. P541 adds a diagram view to proof steps, its tool schema and renderer. The P539 fold
  moves the Studio DB as a whole file, so a new Studio migration applies wherever the file sits.
  P541 reads nothing P539 changes.
- Migration owner: `studiostorage/migrations` P541 (rebuild of `proof_steps` and `proof_view_state`
  for the new view value, same pattern as `0048_p426_proof_chart_view.sql`); `storage/migrations`
  P539 (only if its runs-row copy needs one).

| Shared file | P539 edit | P541 edit |
|---|---|---|
| `internal/flows/proofflow/helpers_test.go` | line 51 `app.KiraHome` helper token path to the Space home | none; new helpers go in a new file (`diagram_test.go`) |
| `tests/ui/support/mockRuntime.ts` | drop `StudioConfigFiles*` entries | none; diagram fixtures in `tests/ui/studio/support/proof.ts` |
| `internal/appwire/studio_build.go` | runs stack, dirs, config files (lines 41-80, 173) | only if proof deps change (lines 122-129) |
| `docs/ARCHITECTURE.md` | home fold sections, Known open item | proof section |

- Not shared: `dbmcp/server.go` (P539), `dbmcp/proof_tools.go` (P541), `internal/llmprompts/registry.go` (P541), `package.json`/`bun.lock` (P541, Mermaid).
- Landing: P539, then P541.

## B3: P540 (config subfolder), P542 (proof chain)

- Why independent: P540 moves YAML folders and adds one config-root resolver (`configfs`,
  `connfiles`, `apifiles`, `adeflow` dirs, `repocfg`, `agentcfg`, appwire joins). P542 links proof
  to tasks and shows proof, links and handovers as a chain in the details panel. Neither reads the
  other's data.
- Migration owner: `studiostorage/migrations` P542 (proof to task link); `storage/migrations` P542
  if its link lives in the Space DB. P540's layout move is a filesystem migration, not SQL.

| Shared file | P540 edit | P542 edit |
|---|---|---|
| `internal/appwire/appwire.go` | config folder map and home joins (around lines 543, 566, 795) | proof wiring into the task board deps, own hunk |
| `internal/appwire/studio_build.go` | `connections`, `api` dirs (lines 60, 80, 168, 173) | `proofDeps` (lines 122-129) |
| `docs/ARCHITECTURE.md` | config subfolder section | proof chain section |

- Not shared: `internal/llmprompts/registry.go` (P542, terse-handover rule), `frontend/src/ade/v2/**` (P542), `dbmcp/proof_tools.go` (P542).
- Landing: P540, then P542.

## B4: P543 (unified notes), P548 (Studio MCP option in steps)

- Why independent: P543 reshapes notes and handovers (Space `ade_task_handovers`, `ade/handover.go`,
  notes components, stage handover prompt). P548 adds a step field and per-session DB MCP access
  (`adeflow` options and parser, step launch, `mcpauth`, `dbmcp` auth, `mcpinstall`, step
  inspector). No file overlap beyond additive struct fields; no ordering.
- Migration owner: `storage/migrations` P543. P548 keeps its session credential in memory; if its
  plan needs a table it stops and moves to B5.

| Shared file | P543 edit | P548 edit |
|---|---|---|
| `internal/bridge/adewire/wire.go` | `Handover` fields | `WorkflowStep` field |
| `frontend/src/ade/v2/wire.ts` | `Handover` type | `WorkflowStep` type |
| `internal/llmprompts/registry.go` | stage handover entries | DB MCP option text, if any; separate entries |
| i18n catalogs | notes keys | step inspector keys |

- Landing: P543, then P548.

## B5: P544 (DB merge), P545 (timeline toggle)

- Why independent: P544 is backend storage, wiring and the Studio settings and op-log frontend
  (`studio/state/*`, `SettingsDialog`, `OperationsDock`, `createOpLogStore`). P545 is the details
  list only (`AdeTaskTimeline.vue`, `taskTimeline.ts`, the chain component from P542/P543,
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
