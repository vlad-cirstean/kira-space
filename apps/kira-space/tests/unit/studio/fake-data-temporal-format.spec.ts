// formatTemporal's ISO-8601-with-`Z` fall-through is correct
// for Postgres and harmless for SQLite, but wrong for the mysql family and ClickHouse, whose
// datetime/timestamp/year columns reject or mangle a trailing `Z` (in strict mode, an outright
// "Incorrect datetime value"; a `year` column against a full ISO string is unambiguous — there is
// no reading under which it works).
// generate.ts imports bridge/data.ts, which reaches '/wails/runtime.js' at module scope — this has
// to be a dynamic import(), after @workbench/testing/unit/window's mock.module registration has run
// (the same pattern row-values-visible-span.spec.ts already uses).
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';

const { formatTemporal } = await import('../../../frontend/src/studio/views/grid/fakeData/generate');

const SAMPLE = new Date(Date.UTC(2024, 4, 1, 10, 20, 30, 123)); // 2024-05-01T10:20:30.123Z

describe('formatTemporal', () => {
  test('date and time columns are dialect-independent (already just a slice of the ISO string)', () => {
    expect(formatTemporal(SAMPLE, 'date', 'mysql')).toBe('2024-05-01');
    expect(formatTemporal(SAMPLE, 'time', 'postgres')).toBe('10:20:30');
  });

  test('postgres/sqlite keep full ISO-8601 with a trailing Z', () => {
    expect(formatTemporal(SAMPLE, 'timestamp without time zone', 'postgres')).toBe(
      '2024-05-01T10:20:30.123Z',
    );
    expect(formatTemporal(SAMPLE, 'datetime', 'sqlite')).toBe('2024-05-01T10:20:30.123Z');
    expect(formatTemporal(SAMPLE, 'datetime', undefined)).toBe('2024-05-01T10:20:30.123Z');
  });

  test("mysql/mariadb datetime and timestamp use MySQL's own space-separated text format, no T/Z", () => {
    expect(formatTemporal(SAMPLE, 'datetime', 'mysql')).toBe('2024-05-01 10:20:30');
    expect(formatTemporal(SAMPLE, 'timestamp', 'mysql')).toBe('2024-05-01 10:20:30');
  });

  test('mysql year is a bare 4-digit year, not an ISO string', () => {
    expect(formatTemporal(SAMPLE, 'year', 'mysql')).toBe('2024');
    expect(formatTemporal(SAMPLE, 'year(4)', 'mysql')).toBe('2024');
  });

  test('clickhouse DateTime/DateTime64 use the same space-separated text format', () => {
    expect(formatTemporal(SAMPLE, 'DateTime', 'clickhouse')).toBe('2024-05-01 10:20:30');
    expect(formatTemporal(SAMPLE, "DateTime64(3, 'UTC')", 'clickhouse')).toBe(
      '2024-05-01 10:20:30',
    );
  });
});
