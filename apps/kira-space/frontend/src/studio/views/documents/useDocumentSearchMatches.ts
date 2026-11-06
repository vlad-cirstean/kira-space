import { computed } from 'vue';
import { searchState as docSearchState, previewLineFor } from './search';

interface Range {
  start: number;
  end: number;
}

export interface PreviewSegment {
  text: string;
  match: boolean;
}

export function useDocumentSearchMatches(tabId: () => string) {
  let lastMatches: unknown = null;
  let lastByRow: Map<number, Range[]> | null = null;
  const matchIndex = computed(() => {
    const entry = docSearchState[tabId()];
    if (!entry) return null;

    let byRow: Map<number, Range[]>;
    if (entry.matches === lastMatches && lastByRow) {
      byRow = lastByRow;
    } else {
      byRow = new Map<number, Range[]>();
      for (const m of entry.matches) {
        const list = byRow.get(m.row);
        if (list) list.push(m);
        else byRow.set(m.row, [m]);
      }
      lastMatches = entry.matches;
      lastByRow = byRow;
    }

    return { byRow, current: entry.index >= 0 ? entry.matches[entry.index] : undefined };
  });

  function isSearchMatch(row: number): boolean {
    return matchIndex.value?.byRow.has(row) ?? false;
  }
  function isCurrentSearchMatch(row: number): boolean {
    return matchIndex.value?.current?.row === row;
  }

  // Match.start/end are offsets into exactly previewLineFor(body), so no separate computation to sync.
  function previewSegments(row: number, body: string): PreviewSegment[] {
    const matches = matchIndex.value?.byRow.get(row);
    const line = previewLineFor(body);
    if (!matches || matches.length === 0) return [{ text: line, match: false }];
    const segments: PreviewSegment[] = [];
    let cursor = 0;
    for (const m of matches) {
      if (m.start > cursor) segments.push({ text: line.slice(cursor, m.start), match: false });
      segments.push({ text: line.slice(m.start, m.end), match: true });
      cursor = m.end;
    }
    if (cursor < line.length) segments.push({ text: line.slice(cursor), match: false });
    return segments;
  }

  return { isSearchMatch, isCurrentSearchMatch, previewSegments };
}
