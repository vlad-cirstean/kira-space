// fileTreeModel.ts's buildTree sorted with a fresh `localeCompare(..., { sensitivity: 'base' })`
// call per comparison -- measured 20x slower than one hoisted Intl.Collator at scale (200k files,
// the MaxListedFiles cap: 3,760ms -> 322ms). This locks in that the visible ordering is unchanged
// (directories first, case-insensitive collation) after switching to the shared collator.
import { describe, expect, test } from 'bun:test';
import {
  buildTree,
  type FileTreeRowVm,
  flatten,
} from '../../frontend/src/ade/v2/files/fileTreeModel';

describe('C13-4: tree sort order after the Intl.Collator hoist', () => {
  test('directories before files, each case-insensitive/locale-collated', () => {
    const paths = [
      'Banana.ts',
      'apple.ts',
      'zebra/index.ts',
      'Zebra/other.ts', // buildTree keys dirs case-sensitively, so this is a distinct sibling node
      'étude.ts',
      'file10.ts',
      'file2.ts',
      '_hidden.ts',
      '.env',
      'Apple/readme.md',
    ];
    const rows: FileTreeRowVm[] = [];
    flatten(buildTree(paths), 0, new Set(), {}, '', rows);
    const topLevel = rows.filter((r) => r.depth === 0);

    // Directories (Apple/, zebra/, Zebra/) sort before files, and within each group names collate
    // case-insensitively.
    const dirNames = topLevel.filter((r) => r.isDir).map((r) => r.name);
    const fileNames = topLevel.filter((r) => !r.isDir).map((r) => r.name);
    expect(topLevel.findIndex((r) => r.isDir)).toBeLessThan(topLevel.findIndex((r) => !r.isDir));
    expect(dirNames).toEqual(
      [...dirNames].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' })),
    );
    expect(fileNames).toEqual([
      '_hidden.ts',
      '.env',
      'apple.ts',
      'Banana.ts',
      'étude.ts',
      'file2.ts',
      'file10.ts',
    ]);
  });
});
