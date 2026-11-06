import { SlickHybridSelectionModel } from 'slickgrid';
import type { KiraSlickGrid } from './kiraSlickGrid';
import type { ScrollVelocityTracker } from './scrollVelocity';

// P107 I2-38: SlickGridHost.vue's and ConsoleSlickGrid.vue's own identical
// SlickHybridSelectionModel construction, and grid.velocity/lastScrollEventAt/scrollEventSeq
// wiring (comment there: "grid.velocity/lastScrollEventAt/scrollEventSeq are wired the same
// four-field way").

/** Near-exact match for this app's four selection kinds; the gutter is the one
 *  rowSelectColumnIds entry (clicking it selects the row). enableMultiSelection: false is a
 *  parity choice, not a limitation — multi-cell disjoint selection has no consumer (Selection has
 *  no shape for it). showDragHandle: false — no Excel-style fill affordance. */
export function createHybridSelectionModel(gutterField: string): SlickHybridSelectionModel {
  return new SlickHybridSelectionModel({
    selectionType: 'mixed',
    rowSelectColumnIds: [gutterField],
    selectActiveCell: true,
    selectActiveRow: true,
    dragToSelect: true,
    autoScrollWhenDrag: true,
    enableMultiSelection: false,
    showDragHandle: false,
  });
}

/** Wires the host's own scroll-velocity sampler into the three fields KiraSlickGrid's row-range
 *  override reads (see its own doc comments): the raw `velocity` reader, `lastScrollEventAt`
 *  (the chase quiescence gate) and `scrollEventSeq` (the per-frame
 *  gate — the host's own counter, not the tracker's). */
export function attachScrollProbes(
  grid: KiraSlickGrid,
  tracker: ScrollVelocityTracker,
  scrollEventSeq: () => number,
): void {
  grid.velocity = tracker.velocity;
  grid.lastScrollEventAt = () => tracker.lastScrollEventAt();
  grid.scrollEventSeq = scrollEventSeq;
}
