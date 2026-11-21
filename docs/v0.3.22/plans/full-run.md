# v0.3.22 non-UI full run

Base: chapter/v0.3.22 at 9df511850. Scope: Go (rest, containers, flows), typecheck, lint, knip, site, package, bun unit tests.

## Results

| Suite | Result |
| --- | --- |
| `bun run lint` (biome, N1-N19, copy, prompts, docs) | pass |
| `bun run typecheck` (10 projects) | pass, 0 errors |
| `bun run test:unit` | 2410 pass, 0 fail, 200 files |
| `bun run verify:packaging` | pass (artifact checks skip: no .app/.dmg) |
| `bun run lint:dead` (knip) | fail, fixed (F1) |

## Findings

### F1 knip: unused export `MODULE_ACTIONS`

`apps/kira-space/frontend/src/shortcuts/moduleActions.ts` exported `MODULE_ACTIONS`, used only inside the file. Cause: P712 (`0c266bcfb`). Fix: drop `export`.
