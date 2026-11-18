import { compareNatural } from '@workbench/behaviour/match';
import { documentFromValue } from './ejson';

export interface PageStatsInput {
  data: Uint8Array;
  offsets: Uint32Array;
}

export interface PageStats {
  /** Top-level line count per row; -1 when the body has no parseable root (raw fallback). */
  lineCounts: Int32Array;
  /** Every top-level field name on the page except `_id`, natural order. */
  fieldNames: string[];
}

/** One decode and one `JSON.parse` per body, for the whole page, off the main thread. */
export function documentPageStats({ data, offsets }: PageStatsInput): PageStats {
  const rows = Math.max(0, offsets.length - 1);
  const lineCounts = new Int32Array(rows);
  const names = new Set<string>();
  const decoder = new TextDecoder();
  for (let row = 0; row < rows; row++) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(decoder.decode(data.subarray(offsets[row], offsets[row + 1])));
    } catch {
      lineCounts[row] = -1;
      continue;
    }
    lineCounts[row] = documentFromValue(parsed)?.children.length ?? -1;
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) continue;
    for (const key of Object.keys(parsed as Record<string, unknown>)) {
      if (key !== '_id') names.add(key);
    }
  }
  return { lineCounts, fieldNames: [...names].sort(compareNatural) };
}
