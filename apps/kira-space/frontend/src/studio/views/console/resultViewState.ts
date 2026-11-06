import type { ProofViewState } from '@shared/domain/proof';

/** What a result pane saves and restores: scroll offsets, the highlighted cell or row, expanded
 *  documents and column widths. A caller opts in by passing `viewState` to ConsoleResultGrid. */
export type ResultViewState = Omit<ProofViewState, 'queryScrollTop' | 'chart'>;

/** The state of a pane nobody has scrolled or clicked yet. */
export const EMPTY_RESULT_VIEW_STATE: ResultViewState = Object.freeze({
  resultScrollTop: 0,
  resultScrollLeft: 0,
  selection: null,
  selectedRow: null,
  expanded: [],
  columnWidths: {},
});
