# v0.2.31

Branch `chapter/v0.2.31`. First `P` is 505 (highest in v0.2.30 is P504), last is P505.

Source: user request. One phase. No code review (user waived). P505 is the last phase, so it also closes the chapter: Opus plan, Sonnet implementation, one full suite run, then hand-written close and PR.

| Phase | Title | Status |
|---|---|---|
| P505 | Flat minimalist redesign | Todo |

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
- Result: pending.
