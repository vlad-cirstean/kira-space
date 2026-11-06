// P21 round 3 performance finding 2: findRanges(doc, query) walked the whole document with no
// memoisation, and was called for the *same* (doc, query) pair up to five times per keystroke
// (ResponseFindBar.vue's matchCounts + scrollToCurrent, ResponsePane.vue's perTargetHighlighters,
// rangeHighlightPlugin's own constructor/update). The fix hoists the actual match-position walk
// behind a small bounded cache — `currentIndex` only relabels which position gets the "current"
// class, so it never needs to bust the cache. findRanges.ts imports nothing that reaches
// window/bridge, so no window stub is needed.
//
// P28 D11: the probe below counts RegExp.prototype.exec calls rather than String toLowerCase ones.
// The walk is a compiled pattern now (editor/searchPattern.ts, shared with the data views' own
// scanner) instead of a lowercased indexOf loop, so toLowerCase is no longer called at all and an
// instrumented copy of it counted zero for both a hit and a miss — i.e. the old probe would have
// passed vacuously whether or not the cache still worked. `exec` is what the walk actually does:
// one call per match plus one that returns null, so a full walk is (matches + 1) and a cache hit is
// exactly 0. The cache's own contract — keyed on (doc, query, options), currentIndex-insensitive,
// bounded at 4 — is unchanged and is what these three tests still assert.
import { describe, expect, test } from 'bun:test';
import { findRanges } from '../../../frontend/src/studio/editor/findRanges';

// Each test uses its own distinct doc text so the module-level position cache (shared across every
// test in this file, by design — it's the same cache the real app shares across calls) never lets
// one test's cache entry answer for another's.
function uniqueDoc(seed: string): string {
  return `${seed} needle here, and a second needle later, plus NEEDLE in caps.`;
}

describe('findRanges correctness (unchanged by the memoisation)', () => {
  test('finds every case-insensitive occurrence, ascending by position', () => {
    const doc = uniqueDoc('correctness-1');
    const ranges = findRanges(doc, 'needle');
    expect(ranges).toHaveLength(3);
    expect(ranges.every((r) => r.class === 'kira-ed-find-match')).toBe(true);
    expect(ranges[0]?.from).toBeLessThan(ranges[1]?.from ?? 0);
    expect(ranges[1]?.from).toBeLessThan(ranges[2]?.from ?? 0);
    for (const r of ranges) {
      expect(doc.slice(r.from, r.to).toLowerCase()).toBe('needle');
    }
  });

  test('currentIndex marks exactly one range as current, by position order', () => {
    const doc = uniqueDoc('correctness-2');
    const ranges = findRanges(doc, 'needle', 1);
    expect(ranges.map((r) => r.class)).toEqual([
      'kira-ed-find-match',
      'kira-ed-find-match-current',
      'kira-ed-find-match',
    ]);
  });

  test('empty query returns no matches', () => {
    expect(findRanges(uniqueDoc('correctness-3'), '')).toEqual([]);
  });

  test('a query with no occurrence returns no matches', () => {
    expect(findRanges(uniqueDoc('correctness-4'), 'zzz-not-present')).toEqual([]);
  });

  test('an out-of-range currentIndex marks nothing current (still valid ranges)', () => {
    const doc = uniqueDoc('correctness-5');
    const ranges = findRanges(doc, 'needle', 99);
    expect(ranges.every((r) => r.class === 'kira-ed-find-match')).toBe(true);
  });
});

describe('findRanges memoisation (finding 2)', () => {
  test('a second call with the same (doc, query) does not re-walk the document', () => {
    const doc = uniqueDoc('memo-1');
    const TARGET = doc;
    let calls = 0;
    const original = RegExp.prototype.exec;
    // biome-ignore lint/suspicious/noExplicitAny: instrumenting a built-in for one assertion
    (RegExp.prototype as any).exec = function (this: RegExp, str: string) {
      if (str === TARGET) calls++;
      return original.call(this, str);
    };
    try {
      findRanges(doc, 'needle');
      // Three matches, so three exec calls that match plus one that returns null.
      expect(calls).toBe(4); // the first call actually walks the document
      findRanges(doc, 'needle');
      expect(calls).toBe(4); // the second call for the same pair reuses the cached positions
      findRanges(doc, 'needle', 2); // a different currentIndex is still a cache hit
      expect(calls).toBe(4);
    } finally {
      RegExp.prototype.exec = original;
    }
  });

  test('the same (doc, query) under different options is a cache miss', () => {
    const doc = uniqueDoc('memo-options');
    const TARGET = doc;
    let calls = 0;
    const original = RegExp.prototype.exec;
    // biome-ignore lint/suspicious/noExplicitAny: instrumenting a built-in for one assertion
    (RegExp.prototype as any).exec = function (this: RegExp, str: string) {
      if (str === TARGET) calls++;
      return original.call(this, str);
    };
    try {
      findRanges(doc, 'needle', undefined, { matchCase: false, wholeWord: false, regex: false });
      const afterFirst = calls;
      expect(afterFirst).toBeGreaterThan(0);
      // Match-case on finds only the two lowercase occurrences, not the NEEDLE in caps — a
      // different result set, so serving the previous entry would be wrong, not just slower.
      const cased = findRanges(doc, 'needle', undefined, {
        matchCase: true,
        wholeWord: false,
        regex: false,
      });
      expect(calls).toBeGreaterThan(afterFirst);
      expect(cased).toHaveLength(2);
    } finally {
      RegExp.prototype.exec = original;
    }
  });

  test('a different query on the same doc is a cache miss (re-walks)', () => {
    const doc = uniqueDoc('memo-2');
    const TARGET = doc;
    let calls = 0;
    const original = RegExp.prototype.exec;
    // biome-ignore lint/suspicious/noExplicitAny: instrumenting a built-in for one assertion
    (RegExp.prototype as any).exec = function (this: RegExp, str: string) {
      if (str === TARGET) calls++;
      return original.call(this, str);
    };
    try {
      findRanges(doc, 'needle');
      const afterFirst = calls;
      expect(afterFirst).toBeGreaterThan(0);
      findRanges(doc, 'second');
      expect(calls).toBeGreaterThan(afterFirst);
    } finally {
      RegExp.prototype.exec = original;
    }
  });

  test('the position cache is bounded — a 5th distinct (doc, query) evicts the oldest entry', () => {
    const docs = Array.from({ length: 5 }, (_, i) => uniqueDoc(`memo-bounded-${i}`));
    for (const d of docs) findRanges(d, 'needle');

    const first = docs[0] ?? '';
    const TARGET = first;
    let calls = 0;
    const original = RegExp.prototype.exec;
    // biome-ignore lint/suspicious/noExplicitAny: instrumenting a built-in for one assertion
    (RegExp.prototype as any).exec = function (this: RegExp, str: string) {
      if (str === TARGET) calls++;
      return original.call(this, str);
    };
    try {
      // The cache holds at most 4 entries, so the 1st doc's entry was evicted by the 5th call
      // above — asking for it again must re-walk it, not serve a stale/absent cache slot.
      findRanges(first, 'needle');
      expect(calls).toBeGreaterThan(0);
    } finally {
      RegExp.prototype.exec = original;
    }
  });
});
