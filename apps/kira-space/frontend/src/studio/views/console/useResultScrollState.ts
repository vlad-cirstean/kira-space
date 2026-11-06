import type { Page } from '@shared/protocol/page';
import { nextTick, onMounted } from 'vue';
import type { ResultViewState } from './resultViewState';

interface Options {
  viewState: () => ResultViewState | null | undefined;
  page: () => Page | null | undefined;
  /** The scroller of the mounted document or key-value list. */
  scrollEl: () => HTMLElement | null;
  selectedRow: () => number | null;
  expandedIds: () => string[];
  onState: (state: ResultViewState) => void;
  onReady: () => void;
}

/** View-state persistence for the document and key-value lists of ConsoleResultGrid: reports the
 *  scroll offsets, highlighted row and expansion, and restores the saved offsets once mounted. The
 *  tabular grid does its own. `viewState` undefined means the caller keeps none. */
export function useResultScrollState(o: Options): { report: () => void } {
  let restoring = false;

  function report(): void {
    if (o.viewState() === undefined || restoring || o.page()?.kind === 'tabular') return;
    const el = o.scrollEl();
    o.onState({
      resultScrollTop: el?.scrollTop ?? 0,
      resultScrollLeft: el?.scrollLeft ?? 0,
      selection: null,
      selectedRow: o.selectedRow(),
      expanded: o.expandedIds(),
      columnWidths: {},
    });
  }

  onMounted(async () => {
    const page = o.page();
    if (!page || page.rowCount === 0) {
      o.onReady();
      return;
    }
    // The tabular grid reports its own `ready`.
    if (page.kind === 'tabular') return;
    await nextTick();
    const saved = o.viewState();
    const el = o.scrollEl();
    if (el && saved) {
      restoring = true;
      el.scrollTop = saved.resultScrollTop;
      el.scrollLeft = saved.resultScrollLeft;
    }
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        restoring = false;
        o.onReady();
      }),
    );
  });

  return { report };
}
