# v0.2.34

Branch `chapter/v0.2.34`. First `P` is 508 (highest in v0.2.33 is P507), last is P509.

Source: user request. Two phases. P509 is the last phase, so it also closes the chapter: Opus plan, Sonnet implementation per phase, one full suite run after P509 (baselines re-record once), 1 code review round, then hand-written close and PR.

| Phase | Title | Status |
|---|---|---|
| P508 | Flatter, consistent visual pass | Done |
| P509 | Task details: free-form editing, aligned Main tab | Todo |

## Rows

### P508 Flatter, consistent visual pass

- Ask: v0.2.31 palette still reads inconsistent and busy. Make the whole app flatter and consistent across both apps and every shared package.
- Flat means: remove useless separators, dividers, delimiters, redundant borders and boxed containers. Separate with spacing, tone steps and typographic weight instead. Draw on modern minimalist open-source apps (Linear, Zed, Notion, shadcn/ui blocks).
- Colors: audit every surface, text, border, accent and status color. One coherent scale, no near-duplicate greys, no ad-hoc one-off hues. Everything via `packages/theme` tokens.
- Constraints: styling only. No component swaps, no features. Keep proportions and condensed density. Tailwind/shadcn tokens, no new scoped `<style>` blocks, no hard-coded colors outside tokens.
- Tests: no new tests. Baselines re-record once in the chapter's single full run, after `fc-match sans-serif` prints Inter.
- Result: pending.

### P509 Task details: free-form editing, aligned Main tab

- Ask 1, details panel: adding and editing items is restricted and cumbersome. Make it Notion-like. Items keep their categories, but editing is as simple as typing text or pasting a link. Paste a URL, it is recognized and filed under the right category. No multi-step forms or dialogs for the common case.
- Ask 2, Main tab of task details: rows look different, are unaligned and hard to follow, and tire the eye. Give every row one shared layout (same columns, alignment, spacing, type scale, icon size), so the tab scans top to bottom.
- Frontend plus whatever bridge support free-form add/edit needs. New feature tests follow the IPC-split rule: flow test and UI spec for the same scenario.
- Close (same phase, after full suite run and 1 code review round, hand-written per CLAUDE.md "Closing a chapter"): Done rows and Results in `SPEC.md`, `docs/HISTORY.md` block, `git rm -r docs/v0.2.34/`, push, PR to `main`.
- Result: pending.
