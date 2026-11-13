# v0.3.12

Restore Plan/task list UI to its look before v0.3.9. Reference: tag `v0.3.8`. Highest prior phase: P624. User complaints after v0.3.9 to v0.3.11:

1. Task cards are entirely colored (full-card tone fill). Not ok. Restore prior look.
2. Plan page top bar (size/header) does not fit on screen. User must make the task list take the whole screen to see it.
3. Branches panel takes too much space initially.
4. Today bar/column was told not to be bright yellow, but now looks like any other day. Must be subtly different, not a strong bright colour.

| Phase | Title | Status |
|-------|-------|--------|
| P625 | Restore task list look to pre-v0.3.9 and subtle Today marker | Todo |

## P625 Restore task list look to pre-v0.3.9 and subtle Today marker

- Scope: Plan page and task list UI under `frontend/src/ade/v2`. Diff against `v0.3.8` first (`git diff v0.3.8..HEAD` on those files) to find what v0.3.9 to v0.3.11 changed.
- Cards: drop full-card tone fill, restore `v0.3.8` card look. Keep any tone as small accent only if `v0.3.8` had it.
- Top bar: restore `v0.3.8` size and header layout so it fits the viewport without making the task list full screen.
- Branches panel: restore `v0.3.8` initial size (collapsed or narrow as before).
- Today marker: brighter grey than other days (subtle lighter neutral grey tint), theme tokens only. No yellow, no strong colour. Works in light and dark.
- Timeline: drop the date gutter beside the cards. Each date shows once as a heading row above its day's cards, trading a little vertical space for horizontal space. Today's heading and group carry the brighter-grey treatment.
- Shell top bar (tabs, capture box, Add): fits narrow windows; every control stays reachable (shrink or wrap, never cut off).
- Keep unrelated v0.3.9 to v0.3.11 changes (performance, feedback) unless they cause a complaint above.
