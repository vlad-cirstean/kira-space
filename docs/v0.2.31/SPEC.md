# v0.2.31

Branch `chapter/v0.2.31`. First `P` is 505 (highest in v0.2.30 is P504), last is P505.

Source: user request. One phase. No code review (user waived). P505 is the last phase, so it also closes the chapter: Opus plan, Sonnet implementation, one full suite run, then hand-written close and PR.

| Phase | Title | Status |
|---|---|---|
| P505 | Flat minimalist redesign | Done |

## Rows

### P505 Flat minimalist redesign

- Ask: modernize the visual design of the entire app. Flatter, minimalist. Scope: Kira Space, Kira Studio, every shared package.
- Colors everywhere: tokens, palette, surfaces, borders, shadows, radii, hover/focus/active states, status colors, dark and light themes. Includes Monaco, terminal and graph themes where they take app colors.
- Constraints:
  - Styling and design only. No component swaps or replacements, no markup restructuring, no feature or behaviour change.
  - Keep proportions, sizes and spacing. Layout stays condensed and efficient.
  - Styles stay in Tailwind/shadcn tokens (`packages/theme`). No new scoped `<style>` blocks, no hard-coded colors outside tokens.
- Tests: no new tests. Visual baselines re-record once, after `fc-match sans-serif` prints Inter, in the single full suite run. Review recorded diffs: only styling may differ.
- Close (same phase, hand-written per CLAUDE.md "Closing a chapter"): Done row and Result in `SPEC.md`, `docs/HISTORY.md` block, `git rm -r docs/v0.2.31/`, push, PR to `main`.
- Result: Flat redesign landed. Single retuned dark palette only: app has no light theme, a theme switch would be a feature (out of scope); light theme needs its own phase. Dialog radius 10px to 8px is the only geometry change. Repaired presentation screenshot flows (pre-existing breakage), fixed a Go window test and the ADE spec reading old tokens. Discovery: Playwright default per-pixel `threshold` 0.2 left visual projects blind to the shift, so all 47 baselines (21 Studio, 26 Space) re-recorded (`3f0a683a`). Full run: lint, lint:dead, lint:go, vet, typecheck clean; unit 1993; Go all ok; Studio 702; Space 1172 (1 in-code skip). Not run: container-backed Go tests and `e2e-real` (no Docker), `ui-timing`, `test:claude*`, `-race`. No code review (user waived).
