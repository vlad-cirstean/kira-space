// views/shared/page/scan.ts's runChunkedScan is a two-phase, frame-driven scheduler
// (the priority window, then an ascending main pass) whose every interesting behaviour is
// defined *by animation-frame boundary* — three separate findings reason
// about exactly this scheduling and none of them got a deterministic assertion, because a
// Playwright test cannot pause between two frames: it can only wait for a settled end state, by
// which point the priority tick, the chunk boundary and the cancel point have all already happened.
// The SQLite seed "completes in one frame, so there is no in-flight scan to navigate"; the
// stale-failure half "has no DOM assertion the suite can make deterministically". A fake
// requestAnimationFrame that hands frames out one at a time turns every one of these into an
// ordinary assertion. scan.ts imports nothing, so — unlike every other spec here — no window stub
// is needed, only the global requestAnimationFrame it calls by name.
import { beforeEach, describe, expect, test } from 'bun:test';
import {
  chunkRowsForColumns,
  eachMatch,
  runChunkedScan,
  type SearchQuery,
} from '../../../frontend/src/studio/views/shared/page/scan';

interface RowMatch {
  row: number;
}

const frames: (() => void)[] = [];
(globalThis as { requestAnimationFrame?: unknown }).requestAnimationFrame = (
  cb: (t: number) => void,
) => {
  frames.push(() => cb(0));
  return frames.length;
};

beforeEach(() => {
  frames.length = 0;
});

function runFrame(): boolean {
  const cb = frames.shift();
  if (!cb) return false;
  cb();
  return true;
}

// Bounded, not a while(true): a queued .then() from a resolved `done` needs one microtask tick to
// run and (for the priority path) schedule its own next requestAnimationFrame call — awaiting a
// resolved Promise between frames lets that happen before the next runFrame() check.
async function drain(maxFrames = 20): Promise<void> {
  for (let i = 0; i < maxFrames; i++) {
    if (!runFrame()) return;
    await Promise.resolve();
  }
}

// Ignores `pattern` entirely and records every row visited — the row-range facts this file checks
// (chunk boundaries, priority-window clamping) don't depend on what a real per-row scan body does,
// only on which rows runChunkedScan itself decided to visit and in what order.
function scanRow(row: number, _pattern: RegExp, out: RowMatch[]): void {
  out.push({ row });
}

const QUERY: SearchQuery = { text: 'x', matchCase: false, wholeWord: false, regex: false };

describe('runChunkedScan — frame semantics (P44 F45)', () => {
  test('2. a 5000-row page publishes rowsScanned exactly [2000, 4000, 5000] and resolves ascending', async () => {
    const rowsScannedTicks: number[] = [];
    const handle = runChunkedScan<RowMatch>(5000, scanRow, QUERY, (_found, rowsScanned) => {
      rowsScannedTicks.push(rowsScanned);
    });
    await drain();
    const { matches, found } = await handle.done;
    expect(rowsScannedTicks).toEqual([2000, 4000, 5000]);
    expect(matches).toHaveLength(5000);
    expect(found).toBe(5000);
    expect(matches.map((m) => m.row)).toEqual(Array.from({ length: 5000 }, (_, i) => i));
  });

  test('2b (P21 round 3 finding 1). chunkRowsForColumns shrinks the per-frame row budget for a wide table', () => {
    // docs/PERF.md's own wide fixture: 60 columns. 2000 rows/frame × 60 cols is the pre-fix
    // per-frame cost this exists to cut down; the post-fix budget must land well under it while
    // never exceeding the original CHUNK_ROWS (a narrow table must not get slower).
    expect(chunkRowsForColumns(60)).toBeLessThan(200);
    expect(chunkRowsForColumns(60)).toBeGreaterThan(0);
    // A 1-2 column page (keyValueRowScanner's own fixed shape, or a narrow table) sees no
    // reduction at all — the multiplier only bites once columnCount is large enough to matter.
    expect(chunkRowsForColumns(1)).toBe(2000);
    expect(chunkRowsForColumns(2)).toBe(2000);
    // Degenerate input (a page reporting 0 columns) falls back to the original flat budget rather
    // than dividing by zero.
    expect(chunkRowsForColumns(0)).toBe(2000);
  });

  test('2c (P21 round 3 finding 1). opts.chunkRows overrides the per-frame row budget end to end', async () => {
    const rowsScannedTicks: number[] = [];
    const handle = runChunkedScan<RowMatch>(
      250,
      scanRow,
      QUERY,
      (_found, rowsScanned) => rowsScannedTicks.push(rowsScanned),
      { chunkRows: 100 },
    );
    await drain();
    await handle.done;
    // 250 rows at 100/frame is three ticks (100, 200, 250), not the default single 2000-row tick
    // a 250-row page would otherwise finish in.
    expect(rowsScannedTicks).toEqual([100, 200, 250]);
  });

  test('3. a priority window runs in its own first frame, publishing rowsScanned === 0 and only its own matches', async () => {
    const ticks: { rowsScanned: number; soFar: readonly RowMatch[] }[] = [];
    runChunkedScan<RowMatch>(
      5000,
      scanRow,
      QUERY,
      (_found, rowsScanned, _totalRows, soFar) => {
        ticks.push({ rowsScanned, soFar: [...soFar] });
      },
      { priority: { from: 4000, to: 4500 } },
    );
    runFrame(); // the priority frame only
    expect(ticks).toHaveLength(1);
    expect(ticks[0]?.rowsScanned).toBe(0);
    expect(ticks[0]?.soFar.map((m) => m.row)).toEqual(
      Array.from({ length: 500 }, (_, i) => 4000 + i),
    );
  });

  test('4. the final array is still strictly ascending from row 0 after a priority pass', async () => {
    const handle = runChunkedScan<RowMatch>(5000, scanRow, QUERY, () => {}, {
      priority: { from: 4000, to: 4500 },
    });
    await drain();
    const { matches } = await handle.done;
    expect(matches.map((m) => m.row)).toEqual(Array.from({ length: 5000 }, (_, i) => i));
  });

  test('5. a priority window is clamped at both ends', () => {
    const soFarSnapshots: number[][] = [];
    runChunkedScan<RowMatch>(
      100,
      scanRow,
      QUERY,
      (_found, _rowsScanned, _totalRows, soFar) => {
        soFarSnapshots.push(soFar.map((m) => m.row));
      },
      { priority: { from: -50, to: 150 } },
    );
    runFrame();
    expect(soFarSnapshots).toHaveLength(1);
    expect(soFarSnapshots[0]).toEqual(Array.from({ length: 100 }, (_, i) => i));
  });

  test('6. an empty or inverted window falls through to the plain path with no priority tick', () => {
    const rowsScannedTicks: number[] = [];
    runChunkedScan<RowMatch>(
      100,
      scanRow,
      QUERY,
      (_found, rowsScanned) => rowsScannedTicks.push(rowsScanned),
      {
        priority: { from: 50, to: 50 },
      },
    );
    runFrame();
    expect(rowsScannedTicks).toEqual([100]); // a main-pass tick, never rowsScanned === 0

    frames.length = 0;
    const invertedTicks: number[] = [];
    runChunkedScan<RowMatch>(
      100,
      scanRow,
      QUERY,
      (_found, rowsScanned) => invertedTicks.push(rowsScanned),
      {
        priority: { from: 80, to: 20 },
      },
    );
    runFrame();
    expect(invertedTicks).toEqual([100]);
  });

  test('7. cancel before the first frame resolves with nothing found', async () => {
    const handle = runChunkedScan<RowMatch>(100, scanRow, QUERY, () => {});
    handle.cancel();
    await drain();
    expect(await handle.done).toEqual({ matches: [], found: 0, matchedRows: [] });
  });

  test('8. cancel mid-scan resolves with what was found so far', async () => {
    const handle = runChunkedScan<RowMatch>(5000, scanRow, QUERY, () => {});
    runFrame(); // one chunk: rows 0..1999
    handle.cancel();
    await drain();
    const { matches, found } = await handle.done;
    expect(matches).toHaveLength(2000);
    expect(found).toBe(2000);
  });

  test('9. an invalid regex throws synchronously, before any frame is scheduled', () => {
    const badQuery: SearchQuery = { text: '(', matchCase: false, wholeWord: false, regex: true };
    expect(() => runChunkedScan<RowMatch>(10, scanRow, badQuery, () => {})).toThrow();
    expect(frames).toHaveLength(0);
  });
});

// runChunkedScan's own cap (opts.cap here, MAX_SCAN_MATCHES in production) — a cap interacting
// with a two-pass scanner whose second pass rebuilds from row 0 and whose contract is that the
// final array is strictly ascending.
describe('runChunkedScan — the match cap (P5 C4/F6)', () => {
  test('11. cap reached inside the priority window: the window keeps counting past it, but its own soFar stays capped', () => {
    const ticks: { found: number; soFar: readonly RowMatch[] }[] = [];
    runChunkedScan<RowMatch>(
      1000,
      scanRow,
      QUERY,
      (found, rowsScanned, _totalRows, soFar) => {
        if (rowsScanned === 0) ticks.push({ found, soFar });
      },
      { priority: { from: 0, to: 200 }, cap: 50 },
    );
    runFrame(); // the priority frame only
    expect(ticks).toHaveLength(1);
    expect(ticks[0]?.found).toBe(200); // the window's true count, uncapped
    expect(ticks[0]?.soFar).toHaveLength(50); // its own returned array, capped
    expect(ticks[0]?.soFar.map((m) => m.row)).toEqual(Array.from({ length: 50 }, (_, i) => i));
  });

  test('12. cap reached mid-main-pass, after a priority window that already found matches under the cap', async () => {
    const handle = runChunkedScan<RowMatch>(5000, scanRow, QUERY, () => {}, {
      priority: { from: 4000, to: 4200 }, // 200 matches — well under the cap, capped separately
      cap: 3000,
    });
    await drain();
    const { matches, found } = await handle.done;
    // The main pass rebuilds from row 0 regardless of the priority window (F30's contract) and
    // caps independently of it — reaching 3000 mid-way through its second 2000-row chunk.
    expect(matches).toHaveLength(3000);
    expect(found).toBe(5000); // the main pass's own true count — every row, uncapped
    expect(matches.map((m) => m.row)).toEqual(Array.from({ length: 3000 }, (_, i) => i));
  });

  test('13. the cap exactly equal to the match count truncates nothing', async () => {
    const handle = runChunkedScan<RowMatch>(100, scanRow, QUERY, () => {}, { cap: 100 });
    await drain();
    const { matches, found } = await handle.done;
    expect(matches).toHaveLength(100);
    expect(found).toBe(100);
    expect(matches.map((m) => m.row)).toEqual(Array.from({ length: 100 }, (_, i) => i));
  });

  test('14. the ascending-order contract holds across a chunk boundary the cap falls inside', async () => {
    // cap (2500) falls inside the *first* 2000-row chunk's own follow-up chunk (2000..3999), the
    // same shape test 12 exercises without a priority window in the mix.
    const handle = runChunkedScan<RowMatch>(5000, scanRow, QUERY, () => {}, { cap: 2500 });
    await drain();
    const { matches, found } = await handle.done;
    expect(matches).toHaveLength(2500);
    expect(found).toBe(5000);
    const rows = matches.map((m) => m.row);
    expect(rows).toEqual(Array.from({ length: 2500 }, (_, i) => i));
    for (let i = 1; i < rows.length; i++) expect(rows[i]).toBeGreaterThan(rows[i - 1] as number);
  });

  test('15. matchedRows stays uncapped so filter mode keeps every matching row', async () => {
    const handle = runChunkedScan<RowMatch>(3, scanRow, QUERY, () => {}, { cap: 2 });
    await drain();
    const { matches, matchedRows } = await handle.done;
    expect(matches).toHaveLength(2);
    expect(matchedRows).toEqual([0, 1, 2]);
  });
});

describe('eachMatch (P44 F45)', () => {
  test('10. a zero-width pattern terminates instead of looping forever, and resets lastIndex', () => {
    const pattern = /x*/g;
    pattern.lastIndex = 5; // stale state from a previous scan — eachMatch must reset it
    const hits: [number, number][] = [];
    eachMatch(pattern, 'abc', (start, end) => hits.push([start, end]));
    expect(hits).toEqual([
      [0, 0],
      [1, 1],
      [2, 2],
      [3, 3],
    ]);
  });

  // F16 (P108 Part 10): eachMatch's own text-length cap — the partial ReDoS mitigation for a
  // user-authored regex against untrusted, unbounded cell content.
  describe('maxLength (F16, P108 Part 10)', () => {
    test('11. omitted maxLength scans the full text, unchanged behavior', () => {
      const hits: [number, number][] = [];
      eachMatch(/a/g, 'aaaaa', (start, end) => hits.push([start, end]));
      expect(hits).toEqual([
        [0, 1],
        [1, 2],
        [2, 3],
        [3, 4],
        [4, 5],
      ]);
    });

    test('12. maxLength shorter than the text only matches within the truncated prefix', () => {
      const hits: [number, number][] = [];
      eachMatch(/a/g, 'aaaaa', (start, end) => hits.push([start, end]), 3);
      expect(hits).toEqual([
        [0, 1],
        [1, 2],
        [2, 3],
      ]);
    });

    test("13. maxLength at or beyond the text's own length is a no-op cap — every match still found", () => {
      const hits: [number, number][] = [];
      eachMatch(/a/g, 'aaa', (start, end) => hits.push([start, end]), 3);
      expect(hits).toEqual([
        [0, 1],
        [1, 2],
        [2, 3],
      ]);
      const hitsLonger: [number, number][] = [];
      eachMatch(/a/g, 'aaa', (start, end) => hitsLonger.push([start, end]), 100);
      expect(hitsLonger).toEqual(hits);
    });

    test('14. a catastrophic-backtracking pattern against text past the cap returns quickly instead of hanging', () => {
      const longText = `${'a'.repeat(50)}!`; // no trailing 'b' — worst case for (a+)+b
      const start = performance.now();
      const hits: [number, number][] = [];
      eachMatch(/(a+)+b/g, longText, (s, e) => hits.push([s, e]), 20);
      expect(performance.now() - start).toBeLessThan(1000);
      expect(hits).toEqual([]);
    });
  });
});
