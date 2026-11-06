// A template handler written as a call expression closing over a v-for/scoped-slot loop
// variable (`@click="onCellClick(row, col)"`) can never be cached by Vue's compiler
// (compiler-core's hasScopeRef bails out on any v-for-scoped reference) — every row/cell gets a
// brand-new wrapper closure allocated on every render the component does for any reason, scroll
// included. Bind a stable, module-scope `on*FromEvent` function instead, and have it recover its
// row/column position from a data-* attribute on the element the listener fired on. This is that
// recovery step, factored out so ConsoleResultGrid/StreamView/KeyValueView don't each carry their
// own copy.
//
// datasetKey is a dataset property name (camelCase — "row" for `data-row`, "rowIndex" for
// `data-row-index`). The walk up parentElement covers a listener bound to a child element (a cell)
// whose row index lives on an ancestor row element, the same shape the deleted
// DataGrid.vue's own onCellNavClickFromEvent needed for its nav button.
export function datasetNumber(target: EventTarget | null, datasetKey: string): number | null {
  let el = target instanceof HTMLElement ? target : null;
  while (el) {
    const raw = el.dataset[datasetKey];
    if (raw !== undefined) {
      const n = Number(raw);
      return Number.isNaN(n) ? null : n;
    }
    el = el.parentElement;
  }
  return null;
}
