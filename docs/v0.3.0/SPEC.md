# v0.3.0

Branch `chapter/v0.3.0`, based on `chapter/v0.2.36` (v0.2.36 not yet merged to main). First `P` is 537 (highest in v0.2.36 is P536), last is P544.

Source: user request. Eight phases: merge both apps into one Kira Space, then proof, notes and timeline rework in the task details panel, then Kira DB audit trail and the Studio MCP workflow option. Close duty after the last phase lands: one full suite run, 2 code review rounds (user ask), hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

Sequencing: P537 first. Every later phase builds on the merged app, so none starts before P537 (all its Parts) is committed. Table order is execution order. P539 to P542 share the task details panel and the notes editor; run one at a time. P543 also depends on v0.2.36 P528 (exemption approvals) having landed.

| Phase | Title | Status |
|---|---|---|
| P537 | Merge Kira Space and Kira Studio into one app named Kira Space | Pending |
| P538 | Mermaid visualiser in the proof tool and proof MCP tool | Pending |
| P539 | Proof as an artifact chain in the task details panel | Pending |
| P540 | Unified notes: user notes and agent handovers in one chain | Pending |
| P541 | Timeline becomes a toggle merged into the same list | Pending |
| P542 | Forward panel edits (diff only) to agents running in the step | Pending |
| P543 | Kira DB audit trail in the details panel | Pending |
| P544 | Studio MCP option in workflow steps | Pending |

## Rows

### P537 Merge Kira Space and Kira Studio into one app named Kira Space

- Ask: merge both apps (`apps/kira-space` and `apps/kira-studio`) into one app named Kira Space. Module order in the app: the existing Kira Space modules first, then the Studio modules.
- Decide in plan: split into Parts if needed (CLAUDE.md Part rule: `P537 Part 1: …`, each with own plan, pass and Result); Go module and `main.go` merge, shared packages already duplicated (`appcore`, `appshell`, `appwire`, `bridge`, `config`, `storage`, `oplog`, `flowharness`, `flows`, `buildinfo`, `layering_test.go`); one frontend, one bridge, binding generation; Taskfile, build and packaging (`docs/PACKAGING.md`), CI and test layout (`tests/`, playwright configs); DB, storage and config migration for existing installs of both apps; what happens to the Studio MCP server (`internal/dbmcp`) and the `kira-db` naming; docs (`ARCHITECTURE.md`, `DEV_ENVIRONMENT.md`, READMEs) updated.
- Plan: `plans/P537.md`.

### P538 Mermaid visualiser in the proof tool and proof MCP tool

- Ask: Mermaid JS visualiser in the proof tool (`internal/proof`, `frontend/src/proof`). The proof MCP tool (`dbmcp/proof_tools.go`) offers that option: the agent can submit a Mermaid diagram as proof.
- Decide in plan: Mermaid library adoption and license check; render safety (sandbox, strict security level, size cap); invalid diagram handling and what the agent is told; storage shape for a diagram proof; tool schema and description text in the llmprompts registry.
- Plan: `plans/P538.md`.

### P539 Proof as an artifact chain in the task details panel

- Ask: proof becomes an artifact in the task details panel. Result is a chain of proof. Links of interest stay at the top, grouped. Everything else is in order (for example diagram, then handover, then other proof) and shown in that order.
- Handover text is terse. Add a short rule for this in the MCP tool description (llmprompts).
- Decide in plan: chain ordering key and model; where the artifact lives relative to `AdeDetailsTab.vue` and `AdeLinkGroups.vue`; how existing proof records migrate into the chain; exact wording and budget of the terse-handover rule.
- Plan: `plans/P539.md`.

### P540 Unified notes: user notes and agent handovers in one chain

- Ask: manual notes unified. The notes panel is hidden by default; a button reveals it so the user can write. User notes appear in the same ordered chain as agent handovers (a handover from the user): one unified model. Adding and editing any note uses the same rich text editor everywhere. Sorted by time, newest first.
- Touches `notes/AdeHandoverList.vue`, `notes/AdeNotesEditor.vue`, `notesExtensions.ts` and the stage handover prompt.
- Decide in plan: data model and migration of existing manual notes and handovers; author marker (user or agent); which editor instances get replaced; hidden-state persistence.
- Plan: `plans/P540.md`.

### P541 Timeline becomes a toggle merged into the same list

- Ask: timeline is no longer a collapsible; it is a toggle. Its items show in the same time order in the same list.
- Touches `details/AdeTaskTimeline.vue` and `taskTimeline.ts`.
- Decide in plan: merged-list item model with P539 and P540 entries; default toggle state and persistence; sort direction consistent with the notes (newest first).
- Plan: `plans/P541.md`.

### P542 Forward panel edits (diff only) to agents running in the step

- Ask: every edit in the panel offers forwarding the change, only the diff, to the agents running in that step.
- Decide in plan: which edits count (notes, links, proof, others in the panel); diff format; how running agents in the step are found and addressed; offer UI (per-edit prompt versus per-save control); no forward when no agent is running.
- Plan: `plans/P542.md`.

### P543 Kira DB audit trail in the details panel

- Ask: when Kira DB is used from a session in Kira Space, every action is recorded in the details panel in chronological order. Record the queries and ops themselves (not the data), time taken, success or failure. Recorded by code, not by the agent.
- Prompts (approvals) are also shown, with accepted or denied, especially exemption ones (v0.2.36 P528).
- Depends on the P537 merge and v0.2.36 P528.
- Decide in plan: capture point in `dbmcp` and link to the session and task; storage and retention; redaction so no row data or bound values land in the trail; how it joins the merged list from P541.
- Plan: `plans/P543.md`.

### P544 Studio MCP option in workflow steps

- Ask: a Studio MCP option in workflow steps. No token; the app manages it automatically. If the Studio MCP server is globally disabled for external use, an agent session started with the DB MCP option still gets it working for that session.
- Touches workflow step editing (`workflows/AdeStepInspector.vue`, `inspector/`) and the MCP server auth (`mcpauth`, `dbmcp`).
- Decide in plan: step schema field and validation; per-session scoped access when the global external switch is off, and that it never opens the server to external clients; session-scoped credential lifecycle handled by the app, never shown to the user; interaction with the P537 naming outcome.
- Plan: `plans/P544.md`.

## Close

2 code review rounds (user ask). After P544: one full suite run with all failures fixed, review rounds, hand-written close per CLAUDE.md, PR.
