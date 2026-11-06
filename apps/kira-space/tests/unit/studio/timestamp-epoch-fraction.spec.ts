//  - parseTimestamp's epoch branches read bare `Number(t)`, which accepts `0x10` (16), `1e9` and
//    `''` (0) as valid epoch text — an empty/blank cell silently read as 1970-01-01 for any caller
//    other than validateFormat, and a hex/exponent spelling silently re-encoded as plain digits on
//    the next edit.
//  - encodeTimestamp's epoch-seconds branch unconditionally `Math.round`ed to the nearest second,
//    so a fractional epoch-seconds value (`1700000000.5`) lost its own sub-second digits on the
//    very first re-encode even when nothing about the value had changed — breaking the "exact
//    inverse" contract the adjacent iso8601 branch already keeps.
import { describe, expect, test } from 'bun:test';
import {
  encodeTimestamp,
  parseTimestamp,
} from '../../../frontend/src/studio/views/shared/celleditor/timestamp';

describe('parseTimestamp epoch validation (F15, P108 Part 10)', () => {
  test('rejects a hex spelling instead of silently reading it as decimal', () => {
    expect(parseTimestamp('epochSeconds', '0x10')).toBeNull();
    expect(parseTimestamp('epochMillis', '0x10')).toBeNull();
  });

  test('rejects an exponent spelling instead of silently reading it as decimal', () => {
    expect(parseTimestamp('epochSeconds', '1e9')).toBeNull();
  });

  test('rejects an empty or blank value instead of reading it as epoch 0 (1970-01-01)', () => {
    expect(parseTimestamp('epochSeconds', '')).toBeNull();
    expect(parseTimestamp('epochSeconds', '   ')).toBeNull();
  });

  test('still accepts a plain signed integer', () => {
    const result = parseTimestamp('epochSeconds', '-100');
    expect(result).not.toBeNull();
    expect(result?.date.getTime()).toBe(-100_000);
  });

  test('accepts a fractional epoch-seconds value and carries the fraction digits into the shape', () => {
    const result = parseTimestamp('epochSeconds', '1700000000.5');
    expect(result).not.toBeNull();
    expect(result?.date.getTime()).toBe(1_700_000_000_500);
    expect(result?.shape.fractionDigits).toBe(1);
    expect(result?.shape.fractionRaw).toBe('5');
  });

  test('a whole-number epoch-seconds value carries no fraction', () => {
    const result = parseTimestamp('epochSeconds', '1700000000');
    expect(result?.shape.fractionDigits).toBe(0);
    expect(result?.shape.fractionRaw).toBe('');
  });
});

describe('encodeTimestamp epoch-seconds fraction round trip (F15, P108 Part 10)', () => {
  test('a fractional epoch-seconds value round-trips to the exact original text unedited', () => {
    const parsed = parseTimestamp('epochSeconds', '1700000000.5');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    expect(encodeTimestamp(parsed.shape, parsed.date)).toBe('1700000000.5');
  });

  test('a sub-millisecond fraction (more digits than Date can hold) still round-trips verbatim', () => {
    const parsed = parseTimestamp('epochSeconds', '1700000000.123456');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    expect(encodeTimestamp(parsed.shape, parsed.date)).toBe('1700000000.123456');
  });

  test('a negative (pre-1970) fractional epoch-seconds value round-trips with the correct sign', () => {
    const parsed = parseTimestamp('epochSeconds', '-1700000000.5');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    expect(encodeTimestamp(parsed.shape, parsed.date)).toBe('-1700000000.5');
  });

  test('editing the millisecond value re-emits the new digits, padded to the original width', () => {
    const parsed = parseTimestamp('epochSeconds', '1700000000.5');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    const edited = new Date(parsed.date.getTime() + 250); // 500ms -> 750ms, a genuine edit
    expect(encodeTimestamp(parsed.shape, edited)).toBe('1700000000.7');
  });

  test('no known fraction (a plain whole-second shape) still rounds to the nearest second, unchanged behavior', () => {
    const parsed = parseTimestamp('epochSeconds', '1700000000');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    const withMs = new Date(parsed.date.getTime() + 600);
    expect(encodeTimestamp(parsed.shape, withMs)).toBe('1700000001');
  });
});
