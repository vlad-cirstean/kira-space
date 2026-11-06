# Kira Space — design system + workbench

Published canvas: https://claude.ai/code/artifact/2f4b49c0-d9fe-45a9-8964-a162eeb876ba

A design canvas covering the whole workbench redesign, built to fix the reason
the current UI reads as inconsistent: colour and radius are tokenised, but there
is no scale for **type, space or control height**, so every component invented
its own. Measured on the current tree: 6 font sizes, ~19 distinct padding pairs,
7 control heights, 8 button class names, and `.icon-button` defined
independently in three files.

Static mockups, not a clickable prototype. Nothing under `apps/kira-space/frontend/src/studio` is
touched by anything in this folder.

## How it is built

The artboards are **generated**, not hand-maintained. `parts/` holds one shared
stylesheet, one icon sheet and one body per screen; `build.mjs` assembles them:

```
node build.mjs        # writes the 16 *.dc.html files from parts/
```

That is the point of the whole arrangement — every artboard loads the *same*
`parts/_style.css` and the *same* `parts/_icons.html`, so two screens cannot
drift apart. Edit `parts/`, never a generated `.dc.html`.

```
parts/
  _head.html _tail.html      page shell
  _style.css                 the entire vocabulary: tokens, primitives, laws
  _icons.html                68 symbols, incl. the eight engine marks
  _sb_*.html                 four connection trees (sql, mongo, redis, stream)
                             plus _sb_nocolor.html — a connection with no colour
  _statusbar.html _ops.html  the two full-width chromes
  _gridrows.html _kinds.html the repeated data blocks
  _kindcss.html _dlgcss.html scoped CSS shared by several bodies
  bodies/<Name>.html         one per artboard
```

## Artboards

**Spec**

- `System` — colour, connection colour (before/after), type, space, size, shape,
  thirteen primitives with every state, sixteen composition laws.

**Screens** (1440×900, the full workbench each time)

- `Main` — SQL data grid, with the filter row, pending edits and Commit/Revert.
- `CellEditor` — the cell detail panel open beside the grid.
- `Documents` — MongoDB collection view.
- `KeyValue` — Redis keyspace view.
- `Stream` — Kafka / SQS message view.
- `Console` — SQL console, on a connection that has **no** colour.
- `Ddl` — generated DDL, read-only.
- `Empty` — a window with no tab open: recent tables and nothing else.
- `FirstRun` — no connections at all; the engine picker is the page.

**Dialogs**

- `NewConnection` — step 1, a grid of engines with their own marks.
- `ConnectionDialog` — step 2, only the chosen engine's fields.
- `SettingsDialog` — Appearance, incl. the one connection palette.
- `FiltersDialog` — tree filters as two checkbox columns.

**Sheets**

- `Toolbars` — every band in every state, and the six view toolbars stacked so
  their identical left edge is visible.
- `Menus` — menus, the command palette, popovers.

## Decisions worth knowing

- **No custom title bar.** macOS draws it. The status bar is the only
  full-width chrome; panel toggles and Settings are icon-only, on its right.
- **Stop always follows Refresh**, disabled when idle — cancelling lives in one
  place instead of appearing only once work starts.
- **Connection colour** is `oklch(0.72 0.09 h)`: eleven evenly-spaced hues plus
  a near-neutral grey, one lightness and one chroma, so nothing out-shouts
  anything else. There is one palette and no intensity setting. **No colour is
  the default**; the 2px rail slot is reserved either way, so assigning one
  moves nothing. It appears as a rail (tree group, tab, and a cap on the view's
  toolbar — not the panel, since the tab strip above it belongs to several
  connections at once) and a dot (view header, operations row) — nowhere else.
- **Progress is a ring in the toolbar**, next to the button that started the
  work, with the elapsed time beside it — not a bar across the top of the view.
  Idle keeps the same slot as a still ring holding the last duration.
- **There is no editor status line.** Everything it used to say already existed
  somewhere: identity in the view header, duration in the toolbar run-state,
  pending edits in the toolbar chip. The app status bar keeps only the caret.
- **First run is one button.** The engine picker lives in the New connection
  dialog and is not repeated on the empty page.
- **The engine marks are drawn here, not vendored.** Eight `currentColor`
  paths in a 16px box — our own redrawings of the Postgres elephant, the
  MariaDB sea lion, the Mongo leaf, the Redis stack, the Kafka K, an SQS
  message, the S3 bucket and the ClickHouse bars — so they obey the icon law
  and take the connection colour. Holes (eyes, the leaf's midrib) are cut with
  `fill-rule="evenodd"`, never painted in the background colour, so they stay
  correct on a hovered or selected row. Shipping the real trademarked marks
  instead is a licensing call; these are drop-in replacements if it is made.
- **The filter row is permanent** — Clear, never close.
- **Search walks the loaded rows only** and never issues a query.

## Implementing it

`tokens.css` gains the type, space and control-height scales; the thirteen
primitives become one shared stylesheet (or small Vue components); the existing
components drop their local button/input/row definitions and consume it.
