import type { Locator, Page } from '@playwright/test';

// The grid locator helpers every grid-touching spec used to redefine locally,
// resolved once for the SlickGrid DOM shape (the only grid engine this app has — the user's own
// call, "no turning back": no incumbent survives to A/B against, so no engine branch here either).
//
// SlickGrid's own `appendRowHtml` clones the row div per frozen pane — with `frozenColumn: 0`
// a row's `.slick-row` exists TWICE (the gutter's left-pane clone and the data cells' right-pane
// clone), both carrying the same `data-row`, while `.slick-cell` exists once. `data-testid=
// "grid-row"` is written onto the right pane only (`SlickGridHost.vue`'s own `onRendered` pass),
// so a plain `[data-testid="grid-row"][data-row="N"]` already resolves to exactly one
// element — only the cell/gutter selectors, which must additionally scope into the correct pane,
// need the pane prefix below.

const RIGHT_PANE = '.grid-canvas-top.grid-canvas-right';

/** The raw CSS string behind `gridCell()` below — exported separately for the handful of call
 *  sites (`measureClickToDom`'s own `click`/`until.selector` options, budgets.spec.ts) that need a
 *  selector *string*, not a `Locator`. `[data-testid="grid-cell"][data-row="N"]` alone is never
 *  valid for SlickGrid's DOM shape: `data-row` is written on the `.slick-row`
 *  (`SlickGridHost.vue`'s own `tagRenderedRows`), never on the cell itself, so it has to be the
 *  row's own ancestor scope,
 *  same as `gridCell()`. */
export function gridCellSelector(row: number, column: string): string {
  return `[data-testid="data-grid"] ${RIGHT_PANE} [data-testid="grid-row"][data-row="${row}"] [data-testid="grid-cell"][data-column="${column}"]`;
}

/** A data cell, addressed the way every other subsystem addresses one: a page row plus a display
 *  column name. */
export function gridCell(page: Page, row: number, column: string): Locator {
  return page.locator(gridCellSelector(row, column));
}

export async function cellText(page: Page, row: number, column: string): Promise<string> {
  return (await gridCell(page, row, column)).innerText();
}

/** The gutter (row-number) cell for one page row. The gutter lives in the frozen left pane, which
 *  carries no `data-testid="grid-row"` of its own — `data-row` is still written on both
 *  panes, and only the left pane has a `[data-testid="grid-gutter-cell"]` descendant, so the
 *  combination is unambiguous without a right/left-pane prefix. */
export function gutterCell(page: Page, row: number): Locator {
  return page.locator(
    `[data-testid="data-grid"] .slick-row[data-row="${row}"] [data-testid="grid-gutter-cell"]`,
  );
}

/** A data column's header cell. */
export function headerCell(page: Page, column: string): Locator {
  return page.locator(`[data-testid="grid-header-cell"][data-column="${column}"]`);
}

/** A whole rendered row, by page row index. Right-pane-only `data-testid="grid-row"` (D10) already
 *  disambiguates the two panes. */
export function gridRow(page: Page, row: number): Locator {
  return page.locator(`[data-testid="grid-row"][data-row="${row}"]`);
}

/** The FK/PK nav button for one cell — the single host-owned button (D11b), moved into the cell
 *  on hover/selection, so a nested locator resolves it correctly once it's showing there. */
export function cellNavButton(page: Page, row: number, column: string): Locator {
  return gridCell(page, row, column).locator('[data-testid="cell-nav-button"]');
}

/** P7 D6: a cell's nav button only appears while its cell carries `.selected` — select it first
 *  the same way a real user's click would, then act on the now-visible button. */
export async function clickCellNav(page: Page, row: number, column: string): Promise<void> {
  await gridCell(page, row, column).click();
  await cellNavButton(page, row, column).click();
}

/** P67: the FK-cell preview popover — a single left-click on an FK nav button opens this instead
 *  of navigating (§3), so this locator is a sibling of `cellNavButton` above, not nested under it
 *  (the popover isn't scoped to any one cell's DOM once open). */
export function fkPreview(page: Page): Locator {
  return page.locator('[data-testid="fk-preview"]');
}

/** Every pending-insert row — `data-testid="grid-row-insert"` is written once per insert row, the
 *  right pane only. */
export function insertRow(page: Page): Locator {
  return page.locator('[data-testid="grid-row-insert"]');
}

/** The element that actually scrolls. The host div (`[data-testid="data-grid"]`) never scrolls —
 *  SlickGrid owns its own internal viewport div, `.slick-viewport-top.slick-viewport-right`. */
export const GRID_SCROLLER_SELECTOR =
  '[data-testid="data-grid"] .slick-viewport-top.slick-viewport-right';

export function gridScroller(page: Page): Locator {
  return page.locator(GRID_SCROLLER_SELECTOR);
}

/** The sort chevron shown over a sorted column's header — `.slick-sort-indicator-asc`/`-desc`
 *  (F8, only created once a column is actually sorted; `setSortColumns` adds the class, it
 *  doesn't toggle a rule). */
export function sortIndicators(page: Page): Locator {
  return page.locator('.slick-sort-indicator-asc, .slick-sort-indicator-desc');
}

/** A NULL cell's own `.cell-null` marker. SlickGrid folds the class onto the cell node itself (the
 *  formatter's own `addClasses`, the "text, never DOM" rule, same reason
 *  `.cell-truncated` is a class + CSS `::after` rather than a child span). `:scope` in a chained
 *  locator selector refers to the parent locator's own matched element(s). */
export function nullMarker(cell: Locator): Locator {
  return cell.locator(':scope.cell-null, :scope .cell-null');
}

/** `viewport.scrollTop += deltaPx`, then count the DOM mutations (`childList` +
 *  `subtree` + `attributes`) that follow. Waits for actual quiescence first — no MutationObserver
 *  records across `quietFrames` consecutive animation frames (default 6, `slick-grid.spec.ts`'s
 *  own convergence horizon) — rather than a fixed sleep, since a scroll into never-rendered
 *  territory converges over several self-scheduled catch-up renders (`kiraSlickGrid.ts`'s re-arming
 *  chase loop), not instantly. The quiet wait and the measurement live in one `page.evaluate`, so
 *  no round trip between them can let a catch-up render slip in unobserved.
 *
 *  Throws (via the in-page `Promise` rejecting) if the grid never goes quiet within `timeoutMs`
 *  (default 10_000) — a real hang, not a false "zero mutations" pass. */
export function mutationsForScroll(
  viewport: Locator,
  deltaPx: number,
  opts?: { quietFrames?: number; timeoutMs?: number },
): Promise<number> {
  const quietFrames = opts?.quietFrames ?? 6;
  const timeoutMs = opts?.timeoutMs ?? 10_000;
  return viewport.evaluate(
    async (el, { deltaPx, quietFrames, timeoutMs }) => {
      const frame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

      // 1. Wait for quiet: no mutation records across `quietFrames` consecutive frames.
      await new Promise<void>((resolve, reject) => {
        const deadline = Date.now() + timeoutMs;
        let clean = 0;
        const settleObserver = new MutationObserver(() => {
          clean = 0;
        });
        settleObserver.observe(el, { childList: true, subtree: true, attributes: true });
        const tick = () => {
          if (clean >= quietFrames) {
            settleObserver.disconnect();
            resolve();
            return;
          }
          if (Date.now() > deadline) {
            settleObserver.disconnect();
            reject(new Error(`mutationsForScroll: grid never went quiet within ${timeoutMs}ms`));
            return;
          }
          clean += 1;
          void frame().then(tick);
        };
        void frame().then(tick);
      });

      // 2. Arm a fresh observer, scroll, measure across two frames.
      let count = 0;
      const measureObserver = new MutationObserver((records) => {
        count += records.length;
      });
      measureObserver.observe(el, { childList: true, subtree: true, attributes: true });
      el.scrollTop += deltaPx;
      await frame();
      await frame();
      measureObserver.disconnect();
      return count;
    },
    { deltaPx, quietFrames, timeoutMs },
  );
}
