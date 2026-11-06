// parseIso8601Shaped/fromEditableText built the instant via
// Date.UTC(Number(y), ...) / new Date(y, ...), which apply ECMAScript's legacy rule mapping a
// `year` argument in 0..99 to 1900 + year — silently remapping a common sentinel like
// `0001-01-01` (Postgres's own "unknown date" convention) to 1901-01-01. encodeTimestamp's own
// unpadded year also broke the round trip for any real year under 1000 (901-01-01 instead of the
// ISO-8601-required 0901-01-01).
import { describe, expect, test } from 'bun:test';
import {
  encodeTimestamp,
  fromEditableText,
  parseTimestamp,
} from '../../../frontend/src/studio/views/shared/celleditor/timestamp';

describe('two-digit and sub-1000 years are not remapped or truncated (finding 10)', () => {
  test('0001-01-01 parses to year 1, not 1901', () => {
    const result = parseTimestamp('iso8601', '0001-01-01');
    expect(result).not.toBeNull();
    expect(result?.date.getUTCFullYear()).toBe(1);
  });

  test('0099-12-31 parses to year 99, not 1999', () => {
    const result = parseTimestamp('iso8601', '0099-12-31');
    expect(result).not.toBeNull();
    expect(result?.date.getUTCFullYear()).toBe(99);
    expect(result?.date.getUTCMonth()).toBe(11);
    expect(result?.date.getUTCDate()).toBe(31);
  });

  test('a datetime with a sub-100 year and a time component also parses without the remap', () => {
    const result = parseTimestamp('iso8601', '0050-06-15T10:20:30Z');
    expect(result).not.toBeNull();
    expect(result?.date.getUTCFullYear()).toBe(50);
    expect(result?.date.getUTCHours()).toBe(10);
  });

  test('encodeTimestamp round-trips a sub-100 year back to the exact original text', () => {
    const parsed = parseTimestamp('iso8601', '0001-01-01');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    expect(encodeTimestamp(parsed.shape, parsed.date)).toBe('0001-01-01');
  });

  test('encodeTimestamp pads a real year under 1000 to 4 digits (901, not 0901, would fail re-parsing)', () => {
    const parsed = parseTimestamp('iso8601', '0901-03-15');
    expect(parsed).not.toBeNull();
    if (!parsed) return;
    const text = encodeTimestamp(parsed.shape, parsed.date);
    expect(text).toBe('0901-03-15');
    // The round trip must itself be a no-op: re-parsing the encoded text recovers the same year.
    const reparsed = parseTimestamp('iso8601', text);
    expect(reparsed?.date.getUTCFullYear()).toBe(901);
  });

  test('opening the cell editor on a sub-100-year sentinel and committing with no edit must not change the value', () => {
    // Simulates the Translate pane: parse the stored value, then re-encode it unchanged — the
    // failure scenario ("if the user touches anything in that pane and commits,
    // the cell is written back as 1901-01-01 — a silent data change caused purely by opening the
    // editor").
    for (const original of ['0001-01-01', '0050-06-15T10:20:30Z', '0099-12-31']) {
      const parsed = parseTimestamp('iso8601', original);
      expect(parsed).not.toBeNull();
      if (!parsed) continue;
      expect(encodeTimestamp(parsed.shape, parsed.date)).toBe(original);
    }
  });

  test("fromEditableText (the Translate pane's own free-text field) does not remap a sub-100 year either", () => {
    const utc = fromEditableText('0001-01-01 00:00:00', 'utc');
    expect(utc?.getUTCFullYear()).toBe(1);

    const local = fromEditableText('0099-12-31 23:59:59', 'local');
    expect(local?.getFullYear()).toBe(99);
  });

  test('an ordinary modern year is unaffected', () => {
    const result = parseTimestamp('iso8601', '2024-05-01T10:20:30.123Z');
    expect(result?.date.getUTCFullYear()).toBe(2024);
    expect(result && encodeTimestamp(result.shape, result.date)).toBe('2024-05-01T10:20:30.123Z');
  });
});
