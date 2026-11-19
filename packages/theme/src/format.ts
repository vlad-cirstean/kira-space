import { t } from '@theme/i18n';
import { formatCount } from '@theme/i18n/number';
import { type FormatTimeAgoOptions, formatTimeAgo } from '@vueuse/core';

// The one place app code turns a number or instant into text. Anything shown to a user as a size,
// a count, a duration, an age, a date-time or a missing value comes from here.

/** A hidden secret value shown as text (`MaskedValue` renders it; composed strings use it). */
export const MASKED_TEXT = '••••••••';

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/** '0 B' / '842 B' / '1.2 KB' / '341 MB': 1024 steps, one decimal under 100, none above. */
export function formatBytes(bytes: number): string {
  let value = Math.max(0, bytes);
  let unit = 0;
  while (unit < BYTE_UNITS.length - 1 && Math.round(value) >= 1024) {
    value /= 1024;
    unit++;
  }
  if (unit === 0) return `${Math.round(value)} B`;
  const text = value.toFixed(value < 99.95 ? 1 : 0);
  if (text === '1024' && unit < BYTE_UNITS.length - 1) return `1.0 ${BYTE_UNITS[unit + 1]}`;
  return `${text} ${BYTE_UNITS[unit]}`;
}

/** One decimal under 100%, whole numbers above (multi-core CPU can pass 100%). */
export function formatPercent(n: number): string {
  return n >= 100 ? `${Math.round(n)}%` : `${n.toFixed(1)}%`;
}

export { SPINNER_DELAY } from '@theme/timing';
export { formatCount };

/** A finished duration: '0.42ms' / '240ms' / '3.4s' / '2m 5s' / '1h 2m'. */
export function formatDuration(ms: number): string {
  if (ms < 1) return t('format.duration.ms', { value: ms.toFixed(2) });
  if (ms < 1000) return t('format.duration.ms', { value: Math.round(ms) });
  const tenths = Math.round(ms / 100) / 10;
  if (tenths < 60) return t('format.duration.secondsCompact', { seconds: tenths.toFixed(1) });
  const totalSeconds = Math.round(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  if (minutes < 60) {
    return t('format.duration.minutesSeconds', { minutes, seconds: totalSeconds % 60 });
  }
  return t('format.duration.hoursMinutes', {
    hours: Math.floor(minutes / 60),
    minutes: minutes % 60,
  });
}

/** A whole-unit duration for timers and TTLs: '5s' / '2m' / '2m 5s' / '1h 2m' / '6d 23h'. `ceil` rounds a
 *  partial second up (a countdown never shows 0s early); the default floors (an elapsed clock). */
export function formatCompactDuration(ms: number, options: { ceil?: boolean } = {}): string {
  const seconds = Math.max(0, options.ceil ? Math.ceil(ms / 1000) : Math.floor(ms / 1000));
  if (seconds < 60) return t('format.duration.secondsCompact', { seconds });
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  if (minutes < 60) {
    return rest === 0
      ? t('format.relative.unitMinute', { count: minutes })
      : t('format.duration.minutesSeconds', { minutes, seconds: rest });
  }
  const hours = Math.floor(minutes / 60);
  if (hours >= 24) {
    const restHours = hours % 24;
    const days = Math.floor(hours / 24);
    return restHours === 0
      ? t('format.relative.unitDay', { count: days })
      : t('format.duration.daysHours', { days, hours: restHours });
  }
  const restMinutes = minutes % 60;
  return restMinutes === 0
    ? t('format.relative.unitHour', { count: hours })
    : t('format.duration.hoursMinutes', { hours, minutes: restMinutes });
}

const USD_FORMAT = new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD' });

/** An amount in US dollars in the user's locale: '$0.42'. */
export function formatCurrency(usd: number): string {
  return USD_FORMAT.format(usd);
}

/** A running clock: 'm:ss', or 'h:mm:ss' from the first hour. */
export function formatElapsed(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const seconds = String(totalSeconds % 60).padStart(2, '0');
  const minutes = Math.floor(totalSeconds / 60);
  if (minutes < 60) return `${minutes}:${seconds}`;
  return `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, '0')}:${seconds}`;
}

type Instant = number | string | Date;

function toMillis(at: Instant): number {
  return at instanceof Date ? at.getTime() : typeof at === 'number' ? at : new Date(at).getTime();
}

const MINUTE_MS = 60_000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;
const MONTH_MS = 30 * DAY_MS;
const YEAR_MS = 365 * DAY_MS;

type RelativeUnit = 'minute' | 'hour' | 'day' | 'month' | 'year';

/** The one relative-time vocabulary: floor rounding; m, h, d, mo, y; past 'ago', future 'in'. `justNow`
 *  is a plain string in VueUse, so the options resolve per call, not at module load. */
export function relativeTimeOptions(): FormatTimeAgoOptions<RelativeUnit> {
  return {
    rounding: 'floor',
    messages: {
      justNow: t('format.relative.justNow'),
      past: (n: string) => t('format.relative.past', { time: n }),
      future: (n: string) => t('format.relative.future', { time: n }),
      invalid: '',
      minute: (n: number) => t('format.relative.unitMinute', { count: n }),
      hour: (n: number) => t('format.relative.unitHour', { count: n }),
      day: (n: number) => t('format.relative.unitDay', { count: n }),
      month: (n: number) => t('format.age.months', { count: n }),
      year: (n: number) => t('format.age.years', { count: n }),
    },
    units: [
      { max: HOUR_MS, value: MINUTE_MS, name: 'minute' },
      { max: DAY_MS, value: HOUR_MS, name: 'hour' },
      { max: MONTH_MS, value: DAY_MS, name: 'day' },
      { max: YEAR_MS, value: MONTH_MS, name: 'month' },
      { max: Number.POSITIVE_INFINITY, value: YEAR_MS, name: 'year' },
    ],
  };
}

/** 'just now' / '5m ago' / '3h ago' / '2d ago' / '4mo ago' / '1y ago'; a future instant reads 'in 5m'. */
export function formatRelative(at: Instant, now: number = Date.now()): string {
  return formatTimeAgo(new Date(toMillis(at)), relativeTimeOptions(), now);
}

const DATE_TIME_FORMAT = new Intl.DateTimeFormat(undefined, {
  dateStyle: 'medium',
  timeStyle: 'short',
});

/** 'Oct 11, 2026, 4:29 AM' in the user's locale. */
export function formatDateTime(at: Instant): string {
  return DATE_TIME_FORMAT.format(toMillis(at));
}

const WEEKDAY_DATE_FORMAT = new Intl.DateTimeFormat(undefined, {
  weekday: 'short',
  day: 'numeric',
  month: 'short',
});

/** A date without a time, with its weekday: 'Tue, 14 Oct' in the user's locale. */
export function formatWeekdayDate(at: Instant): string {
  return WEEKDAY_DATE_FORMAT.format(toMillis(at));
}

const AGE_MINUTE = 60;
const AGE_HOUR = 60 * AGE_MINUTE;
const AGE_DAY = 24 * AGE_HOUR;
const AGE_MONTH = 30 * AGE_DAY;
const AGE_YEAR = 365 * AGE_DAY;

/** A terse age sized for a narrow column: '<1m' / '5m' / '2h' / '3d' / '4mo' / '1y'. A timestamp
 *  in the future (clock skew) clamps to '<1m'. */
export function formatAgeCompact(timestampSeconds: number, nowMs: number = Date.now()): string {
  const delta = Math.max(0, Math.floor(nowMs / 1000) - timestampSeconds);
  if (delta < AGE_MINUTE) return t('format.age.now');
  if (delta < AGE_HOUR)
    return t('format.relative.unitMinute', { count: Math.floor(delta / AGE_MINUTE) });
  if (delta < AGE_DAY)
    return t('format.relative.unitHour', { count: Math.floor(delta / AGE_HOUR) });
  if (delta < AGE_MONTH)
    return t('format.relative.unitDay', { count: Math.floor(delta / AGE_DAY) });
  if (delta < AGE_YEAR) return t('format.age.months', { count: Math.floor(delta / AGE_MONTH) });
  return t('format.age.years', { count: Math.floor(delta / AGE_YEAR) });
}

// UTC and a fixed locale on purpose: the same commit reads the same for everyone on a team.
const ABSOLUTE_UTC_FORMAT = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'UTC',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
});

/** '2024-03-14 09:41' in UTC; always 16 characters. */
export function formatAbsoluteUtc(timestampSeconds: number): string {
  const parts = ABSOLUTE_UTC_FORMAT.formatToParts(new Date(timestampSeconds * 1000));
  const get = (type: Intl.DateTimeFormatPartTypes): string =>
    parts.find((p) => p.type === type)?.value ?? '';
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}`;
}

/** The window's own IANA zone; 'UTC' when the runtime cannot tell. */
export function localTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

/** 'Tue 09:00' in the zone; a zone Intl rejects falls back to UTC. */
export function formatZonedWeekdayClock(ms: number, timezone: string): string {
  const opts: Intl.DateTimeFormatOptions = {
    weekday: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  };
  try {
    return new Intl.DateTimeFormat(undefined, { ...opts, timeZone: timezone }).format(ms);
  } catch {
    return new Intl.DateTimeFormat(undefined, { ...opts, timeZone: 'UTC' }).format(ms);
  }
}

/** Weekday names in the user's locale, Sunday first. */
export function weekdayNames(style: 'long' | 'short' | 'narrow' = 'short'): string[] {
  const f = new Intl.DateTimeFormat(undefined, { weekday: style, timeZone: 'UTC' });
  return Array.from({ length: 7 }, (_, i) => f.format(Date.UTC(2023, 0, 1 + i)));
}

/** Month names in the user's locale, January first. */
export function monthNames(style: 'long' | 'short' = 'long'): string[] {
  const f = new Intl.DateTimeFormat(undefined, { month: style, timeZone: 'UTC' });
  return Array.from({ length: 12 }, (_, i) => f.format(Date.UTC(2023, i, 1)));
}

/** '04:29:57.123' in local time. */
export function formatClockTime(at: Instant): string {
  const d = new Date(toMillis(at));
  const pad = (n: number, len = 2): string => String(n).padStart(len, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
}

/** The placeholder for a missing value: an em dash, never an empty cell or '-'. */
export function displayValue(value: string | number | null | undefined): string {
  return value === null || value === undefined || value === '' ? '—' : String(value);
}
