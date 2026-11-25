# v0.3.26

Chapter v0.3.26, branch `chapter/v0.3.26`, tag `v0.3.26`. First phase `P756` (highest phase: `P755`,
in `docs/history/07-v0.3.15.md`, chapter v0.3.25).

Code review at chapter end: 1 round.

| # | Phase | Status |
|---|-------|--------|
| P756 | Revert v0.3.25 P754: checkboxes and toggles back to the neutral on state | Todo |
| P757 | Close | Todo |

## P756

Owner dislikes the blue on state of checkboxes, toggles and the keep-awake button. Revert all of
v0.3.25 P754: `--kira-toggle-on` and `--kira-toggle-on-edge` tokens, primitives (Checkbox, Switch,
Toggle, ToggleGroup, SecondaryTabs and others P754 touched), guard N18, keep-awake blue icon and
status dot, UI specs, visual baselines, `ARCHITECTURE.md` text. Restore the pre-P754 neutral look
(`data-[state=on]:bg-select`). Land as a forward change, not a git revert of the squash. Keep every
other v0.3.25 change. Plan lists P754's file set from the `chapter/v0.3.25` history (commits
`28780367d`, `1aa4a0da2`, `f0112b77f`, `787bbbc98`, `6b2a08eeb`).
