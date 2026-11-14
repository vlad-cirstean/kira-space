# v0.3.13: functional verification by user journeys

Goal: verify every feature works as a user uses it, via complex journeys with far-end oracles
(clipboard contents, file on disk, DB row, remote git state), not screen-only assertions. Add or
improve flow tests and UI specs per the IPC-boundary split; fix every product bug found.
First `P` is P626.

| P | Phase | Status |
|---|---|---|
| P626 | Inventory of full history and map of existing e2e coverage (Opus) | Done |
| P627 | Journey imagination: new complex scenarios with definition of correct (Fable, no plan, no implementation) | Done |
| P628 | Synthesis: extend/create decisions, 3 streams plan (Opus) | Todo |
| P629 | Stream A: tests and fixes | Todo |
| P630 | Stream B: tests and fixes | Todo |
| P631 | Stream C: tests and fixes | Todo |
| P632 | Chapter non-UI full run, 2 code review rounds | Todo |
| P633 | Close chapter | Todo |

Deliverables of P626-P628 live in `plans/` (inventory.md, coverage-map.md, journeys-*.md, synthesis.md).
