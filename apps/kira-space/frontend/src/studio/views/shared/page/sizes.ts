import type { PageSize } from '@shared/domain/tabs';

// The same 10/100/1000/10000 + '10'/'100'/'1k'/'10k' literal, once, differing only in each view's
// own testid prefix. `maxPageSize` (Caps' own optional ceiling) filters the list down to what the
// engine can serve — omitting it (or passing null/undefined, the shape `caps.maxPageSize` itself
// takes when absent) is byte-for-byte today's behaviour, which is what the three unchanged call
// sites (grid/documents/keyvalue) rely on.
export function pageSizeOptions(
  testidPrefix: '' | 'document-' | 'keyvalue-' | 'stream-',
  maxPageSize?: number | null,
): { value: PageSize; label: string; testid: string }[] {
  const all = [
    { value: 10, label: '10', testid: `${testidPrefix}page-size-10` },
    { value: 100, label: '100', testid: `${testidPrefix}page-size-100` },
    { value: 1000, label: '1k', testid: `${testidPrefix}page-size-1000` },
    { value: 10000, label: '10k', testid: `${testidPrefix}page-size-10000` },
  ] as const;
  if (maxPageSize == null) return all.slice();
  return all.filter((o) => o.value <= maxPageSize);
}
