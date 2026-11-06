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
| B2 | P539, P542 | B3 starts |
| B3 | P540, P543 | B4 starts |
| B4 | P544, P548 | B5 starts |
| B5 | P541, P545 | B6 starts |
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

Table order holds as dependency order: every phase's dependencies sit above it. Batching runs some
higher phases before lower ones; the user-asked batch exception covers that, so no renumber.
Largest shift: P541 (DB merge) runs in B5, after P544. Reason: P541 folds both migration sequences,
and P542, P543, P544 add schema; finishing their schema first lets P541 plan against the final
tables once. P539 and P540 still land before P541 (strict chain kept). The no-shift alternative
(P541 with P548 in B4, then P544, P545, P546 with P547 alone) needs 7 batches.

## Pairs that cannot run together

- P538 with any later phase it would otherwise pair with. P538 rewrites the shared test harness:
  54 `StudioContract` callers (dbflow, dbmcpflow, proofflow, apiflow, dockerflow, termflow, others),
  `tests/ui/studio` fixtures and mock runtime, Playwright projects. P539 also edits
  `flowharness/contract.go` and the `tests/ui/studio/automations-*` specs P538 repoints. P542 edits
  `proofflow` tests and proof UI specs (both on the Studio harness today). P548 adds `dbmcpflow`
  tests. A new test written against `StudioContract` breaks once P538 lands. So B1 is P538 alone.
- P539, P540, P541: strict chain, never two at once.
- P542 then P543: the chain orders diagram proof first; the diagram view comes from P542.
- P543 and P544: both rework handover display (`AdeHandoverList`, handovers in a chain).
- P545 needs P543 and P544 (merged list of their entries).
- P546 and P547 need P545 (merged list and its edit surfaces). P546 and P545 also restructure the
  same list.
- P541 with P542, P543, P544 or P547: each adds proof, handover or audit schema that P541 folds.
- P539 and P548: both reshape the DB MCP token, helper script and agent registrations
  (`bridge/dbmcp.go`, `internal/mcpauth`, `internal/harness/claude/mcpinstall`).
- P543 and P548: both may add per-session identity to `dbmcp` (proof to task link; session
  credential). P548 runs after P543 and builds on what it added; P547 runs after both and uses it for
  session and task attribution.

## B1: P538

- Why alone: see first bullet above.
- Shared files: none (one stream).

## B2: P539 (home fold), P542 (Mermaid proof)

- Why independent: P539 moves paths, the home, the second runs stack, the config-files twin, the DB
  MCP token. P542 adds a diagram view to proof steps, its tool schema and renderer. The P539 fold
  moves the Studio DB as a whole file, so a new Studio migration applies wherever the file sits.
  P542 reads nothing P539 changes.
- Migration owner: `studiostorage/migrations` P542 (rebuild of `proof_steps` and `proof_view_state`
  for the new view value, same pattern as `0048_p426_proof_chart_view.sql`); `storage/migrations`
  P539 (only if its runs-row copy needs one).

| Shared file | P539 edit | P542 edit |
|---|---|---|
| `internal/flows/proofflow/helpers_test.go` | line 51 `app.KiraHome` helper token path to the Space home | none; new helpers go in a new file (`diagram_test.go`) |
| `tests/ui/support/mockRuntime.ts` | drop `StudioConfigFiles*` entries | none; diagram fixtures in `tests/ui/studio/support/proof.ts` |
| `internal/appwire/studio_build.go` | runs stack, dirs, config files (lines 41-80, 173) | only if proof deps change (lines 122-129) |
| `docs/ARCHITECTURE.md` | home fold sections, Known open item | proof section |

- Not shared: `dbmcp/server.go` (P539), `dbmcp/proof_tools.go` (P542), `internal/llmprompts/registry.go` (P542), `package.json`/`bun.lock` (P542, Mermaid).
- Landing: P539, then P542.

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

## B4: P544 (unified notes), P548 (Studio MCP option in steps)

- Why independent: P544 reshapes notes and handovers (Space `ade_task_handovers`, `ade/handover.go`,
  notes components, stage handover prompt). P548 adds a step field and per-session DB MCP access
  (`adeflow` options and parser, step launch, `mcpauth`, `dbmcp` auth, `mcpinstall`, step
  inspector). No file overlap beyond additive struct fields; no ordering.
- Migration owner: `storage/migrations` P544. P548 keeps its session credential in memory; if its
  plan needs a table it stops and moves to B5.

| Shared file | P544 edit | P548 edit |
|---|---|---|
| `internal/bridge/adewire/wire.go` | `Handover` fields | `WorkflowStep` field |
| `frontend/src/ade/v2/wire.ts` | `Handover` type | `WorkflowStep` type |
| `internal/llmprompts/registry.go` | stage handover entries | DB MCP option text, if any; separate entries |
| i18n catalogs | notes keys | step inspector keys |

- Landing: P544, then P548.

## B5: P541 (DB merge), P545 (timeline toggle)

- Why independent: P541 is backend storage, wiring and the Studio settings and op-log frontend
  (`studio/state/*`, `SettingsDialog`, `OperationsDock`, `createOpLogStore`). P545 is the details
  list only (`AdeTaskTimeline.vue`, `taskTimeline.ts`, the chain component from P543/P544,
  `state/adePanelTabs.ts`). Condition: P545 keeps toggle persistence out of the settings table (a
  Pinia store, as `adePanelTabs` does today); a plan that needs a settings key moves P545 to B6.
- Migration owner: both dirs P541. P545 adds none.

| Shared file | P541 edit | P545 edit |
|---|---|---|
| `tests/ui/support/mockRuntime.ts` | drop `StudioSettings*`, `StudioOps*` entries | none expected; timeline fixtures in its spec |
| i18n catalogs | drop Studio settings and op-log keys | timeline toggle keys |

- Landing: P541, then P545.

## B6: P546 (forward panel edits), P547 (Kira DB audit trail)

- Why independent: P546 hooks the panel's edit paths (notes, links, proof) and delivers a diff to
  the step's running agents (`internal/ade`, `bridge/adetask.go`, agent sessions). P547 captures
  every Kira DB call and approval in `dbmcp`, stores it in the merged DB and adds an entry kind to
  the merged list. P547's prerequisites: v0.2.36 P528 and P537 (landed), P541 (merged DB), P545
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
