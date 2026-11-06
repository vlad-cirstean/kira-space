// ProjectionMenu.vue's pure close-time decision, split into its own module so it has a direct unit
// test (document-projection-menu-close.spec.ts) — mirrors grid/menu.ts's own
// nextProjectionFromSelectedColumns/nextProjectionAfterHidingColumn split for the exact same
// reason (tested at its own boundary, not through setProjection's real load() pipeline).

function setsEqual(a: ReadonlySet<string>, b: ReadonlySet<string>): boolean {
  if (a.size !== b.size) return false;
  for (const v of a) if (!b.has(v)) return false;
  return true;
}

// Returns `undefined` for "don't call setProjection at all" (the untouched-close no-op), distinct
// from `null` ("clear the projection").
//
// Three fixes over the old always-call-setProjection, infer-from-set-size behaviour. (1) A close
// that never touched `selected` is a no-op — it used to always write, resetting pageIndex and
// reloading even when nothing changed, and (worse) silently clearing an active projection
// whenever the projected page's own field set happened to equal that projection (fieldNames is
// itself computed from the *already projected* page, so that equality is common, not evidence of
// intent). `explicitAll` bypasses this skip — a real All press must still clear the projection
// even when its own resulting value happens to be set-equal to the untouched one. (2) "Everything"
// comes only from `explicitAll`, never from a set-size coincidence. (3) An empty selection
// resolves to null (Mongo's own empty-projection behaviour already returns every field; keeping
// `projection: []` disagreed with the edit gate and the badge about what the connection was doing).
export function nextDocumentProjectionOnClose(
  selected: ReadonlySet<string>,
  initialSelected: ReadonlySet<string>,
  fieldNamesLength: number,
  explicitAll: boolean,
): string[] | null | undefined {
  if (!explicitAll && setsEqual(selected, initialSelected)) {
    return undefined;
  }
  if (selected.size === 0) return null;
  const isEverything = explicitAll && selected.size === fieldNamesLength;
  return isEverything ? null : [...selected];
}
