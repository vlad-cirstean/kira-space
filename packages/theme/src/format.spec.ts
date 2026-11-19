import { describe, expect, test } from 'bun:test';
import {
  formatAbsoluteUtc,
  formatAgeCompact,
  formatBytes,
  formatCompactDuration,
  formatCurrency,
  formatDuration,
  formatElapsed,
  formatRelative,
} from './format';

describe('formatBytes rounding edges', () => {
  test('carries into the next unit instead of printing 1024', () => {
    expect(formatBytes(1023.95 * 1024)).toBe('1.0 MB');
    expect(formatBytes(1023)).toBe('1023 B');
    expect(formatBytes(1023.6)).toBe('1.0 KB');
  });

  test('switches from one decimal to none at 99.95', () => {
    expect(formatBytes(99.9 * 1024)).toBe('99.9 KB');
    expect(formatBytes(99.96 * 1024)).toBe('100 KB');
  });

  test('negative clamps to zero; top unit does not carry', () => {
    expect(formatBytes(-5)).toBe('0 B');
    expect(formatBytes(2048 * 1024 ** 4)).toBe('2048 TB');
  });
});

describe('formatDuration unit boundaries', () => {
  test('sub-ms keeps two decimals; ms rounds whole', () => {
    expect(formatDuration(0.424)).toBe('0.42ms');
    expect(formatDuration(0)).toBe('0.00ms');
    expect(formatDuration(0.999)).toBe('1.00ms');
    expect(formatDuration(1)).toBe('1ms');
    expect(formatDuration(240.4)).toBe('240ms');
  });

  test('ms, seconds, minutes, hours', () => {
    expect(formatDuration(999)).toBe('999ms');
    expect(formatDuration(1000)).toBe('1.0s');
    expect(formatDuration(59_949)).toBe('59.9s');
    expect(formatDuration(59_950)).toBe('1m 0s');
    expect(formatDuration(125_000)).toBe('2m 5s');
    expect(formatDuration(3_600_000)).toBe('1h 0m');
    expect(formatDuration(3_725_000)).toBe('1h 2m');
  });
});

describe('formatCompactDuration', () => {
  test('seconds, whole minutes, minutes with seconds, hours', () => {
    expect(formatCompactDuration(0)).toBe('0s');
    expect(formatCompactDuration(5_000)).toBe('5s');
    expect(formatCompactDuration(59_999)).toBe('59s');
    expect(formatCompactDuration(60_000)).toBe('1m');
    expect(formatCompactDuration(125_000)).toBe('2m 5s');
    expect(formatCompactDuration(3_600_000)).toBe('1h');
    expect(formatCompactDuration(3_725_000)).toBe('1h 2m');
  });

  test('days from 24h, minutes dropped', () => {
    expect(formatCompactDuration(86_399_000)).toBe('23h 59m');
    expect(formatCompactDuration(86_400_000)).toBe('1d');
    expect(formatCompactDuration(167 * 3_600_000 + 12 * 60_000)).toBe('6d 23h');
  });

  test('ceil rounds a partial second up so a countdown never shows 0s early', () => {
    expect(formatCompactDuration(100)).toBe('0s');
    expect(formatCompactDuration(100, { ceil: true })).toBe('1s');
    expect(formatCompactDuration(59_001, { ceil: true })).toBe('1m');
  });

  test('negative clamps to zero', () => {
    expect(formatCompactDuration(-5_000)).toBe('0s');
  });
});

describe('formatCurrency', () => {
  test('US dollars with cents, zero and large amounts', () => {
    expect(formatCurrency(0.42)).toMatch(/\$0\.42/);
    expect(formatCurrency(0)).toMatch(/\$0\.00/);
    expect(formatCurrency(1234.5)).toMatch(/\$1,234\.50/);
  });
});

describe('formatElapsed', () => {
  test('m:ss then h:mm:ss from the first hour', () => {
    expect(formatElapsed(59_999)).toBe('0:59');
    expect(formatElapsed(3_599_000)).toBe('59:59');
    expect(formatElapsed(3_600_000)).toBe('1:00:00');
  });
});

describe('formatAbsoluteUtc widest-output invariant', () => {
  test('every rendered string is exactly 16 characters', () => {
    const samples = [
      0,
      Date.UTC(2024, 11, 30, 22, 48) / 1000,
      Date.UTC(2024, 0, 1, 0, 0) / 1000,
      Date.UTC(1999, 8, 9, 9, 9) / 1000,
      Date.UTC(2100, 10, 25, 13, 5) / 1000,
    ];
    for (const s of samples) expect(formatAbsoluteUtc(s)).toHaveLength(16);
  });

  test('matches YYYY-MM-DD HH:MM', () => {
    expect(formatAbsoluteUtc(Date.UTC(2024, 2, 14, 9, 41) / 1000)).toBe('2024-03-14 09:41');
  });
});

describe('formatAgeCompact unit boundaries', () => {
  const now = Date.UTC(2024, 5, 15, 12, 0, 0);
  const sec = now / 1000;
  const age = (delta: number): string => formatAgeCompact(sec - delta, now);

  test('future and under a minute clamp to <1m', () => {
    expect(age(-3600)).toBe('<1m');
    expect(age(0)).toBe('<1m');
    expect(age(59)).toBe('<1m');
  });

  test('minutes, hours, days', () => {
    expect(age(60)).toBe('1m');
    expect(age(59 * 60)).toBe('59m');
    expect(age(3600)).toBe('1h');
    expect(age(23 * 3600)).toBe('23h');
    expect(age(86400)).toBe('1d');
    expect(age(29 * 86400)).toBe('29d');
  });

  test('months, years', () => {
    expect(age(30 * 86400)).toBe('1mo');
    expect(age(11 * 30 * 86400)).toBe('11mo');
    expect(age(365 * 86400)).toBe('1y');
    expect(age(2 * 365 * 86400)).toBe('2y');
  });
});

describe('formatRelative', () => {
  const now = Date.UTC(2026, 5, 1, 12);
  const ago = (ms: number): string => formatRelative(now - ms, now);
  const MIN = 60_000;
  const HOUR = 60 * MIN;
  const DAY = 24 * HOUR;

  test('floors each unit and switches at its boundary', () => {
    expect(ago(59_999)).toBe('just now');
    expect(ago(MIN)).toBe('1m ago');
    expect(ago(119_999)).toBe('1m ago');
    expect(ago(HOUR - 1)).toBe('59m ago');
    expect(ago(HOUR)).toBe('1h ago');
    expect(ago(DAY - 1)).toBe('23h ago');
    expect(ago(DAY)).toBe('1d ago');
  });

  test('months from 30 days, years from 365 days', () => {
    expect(ago(30 * DAY - 1)).toBe('29d ago');
    expect(ago(30 * DAY)).toBe('1mo ago');
    expect(ago(364 * DAY)).toBe('12mo ago');
    expect(ago(365 * DAY)).toBe('1y ago');
    expect(ago(800 * DAY)).toBe('2y ago');
  });

  test('a future instant reads "in"', () => {
    expect(ago(-5 * MIN)).toBe('in 5m');
    expect(ago(-2 * DAY)).toBe('in 2d');
    expect(ago(-30_000)).toBe('just now');
  });
});
