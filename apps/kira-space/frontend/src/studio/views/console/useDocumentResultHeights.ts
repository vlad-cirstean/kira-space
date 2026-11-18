import type { Page } from '@shared/protocol/page';
import { computed, type Ref, watch } from 'vue';
import { rowsOfIds } from '../shared/document/idIndex';
import { useDocumentPageStatsStore } from '../shared/document/pageStatsStore';
import { useDocumentRowsStore } from '../shared/document/rows';
import { useHeightAnchor } from '../shared/document/useHeightAnchor';
import { getPage, pageVersion } from './resultPages';
import { useConsoleViewStore } from './state';

interface Options {
  tabId: () => string;
  pageKey: () => string;
  page: () => Page | null;
  rowIndices: () => readonly number[];
  scrollEl: Ref<HTMLElement | null>;
}

/**
 * Row heights for a console document result. Collapsed rows are `HEAD_H` with no per-row work.
 * Expanded rows are exact for the rendered window (`fillWindow`), counted by the parse worker for
 * the rest of the page, and estimated until it answers: never a whole-page parse on this thread.
 */
export function useDocumentResultHeights({ tabId, pageKey, page, rowIndices, scrollEl }: Options) {
  const rowsStore = useDocumentRowsStore();
  const consoleViewStore = useConsoleViewStore();
  const pageStatsStore = useDocumentPageStatsStore();

  const heights = computed<number[] | undefined>(() => {
    void pageVersion.n;
    void rowsStore.rowsVersion.n;
    void rowsStore.lineCountsVersion.n;
    const expanded = consoleViewStore.resultDocExpandedIds(tabId(), pageKey());
    if (expanded.length === 0) return undefined;
    const current = getPage(pageKey());
    const open = rowsOfIds(current?.kind === 'document' ? current : null, expanded);
    return rowIndices().map((row) => rowsStore.knownRowHeight(pageKey(), row, null, open.has(row)));
  });

  const hasExpanded = computed(
    () => consoleViewStore.resultDocExpandedIds(tabId(), pageKey()).length > 0,
  );

  watch(
    () => [hasExpanded.value, page()] as const,
    ([expanded, current]) => {
      if (expanded && current?.kind === 'document') pageStatsStore.request(pageKey(), current);
    },
    { immediate: true },
  );

  useHeightAnchor(scrollEl, () => heights.value);

  function fillWindow(items: readonly { index: number }[]): void {
    if (!hasExpanded.value) return;
    const list = rowIndices();
    rowsStore.fillLineCounts(
      pageKey(),
      items.flatMap((item) => list[item.index] ?? []),
    );
  }

  return { heights, hasExpanded, fillWindow };
}
