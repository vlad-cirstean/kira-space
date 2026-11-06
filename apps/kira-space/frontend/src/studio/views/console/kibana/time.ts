import type { KibanaTimePreset, KibanaTimeRange, KibanaTimeSelection } from '@shared/domain/kibana';
import { localTimezone } from '@theme/format';

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

const RELATIVE_MS: Partial<Record<KibanaTimePreset, number>> = {
  last15m: 15 * MINUTE,
  last30m: 30 * MINUTE,
  last1h: HOUR,
  last4h: 4 * HOUR,
  last12h: 12 * HOUR,
  last24h: DAY,
  last7d: 7 * DAY,
  last30d: 30 * DAY,
  last90d: 90 * DAY,
  last1y: 365 * DAY,
};

export const KIBANA_RELATIVE_PRESETS = Object.keys(RELATIVE_MS) as KibanaTimePreset[];
export const KIBANA_CALENDAR_PRESETS: KibanaTimePreset[] = [
  'today',
  'thisWeek',
  'thisMonth',
  'thisYear',
];

/** The browser's IANA zone, sent with every request so date math and histogram buckets match. */
export function browserTimeZone(): string {
  return localTimezone();
}

/** First day of the week, 1 = Monday ... 7 = Sunday, from the locale when the engine exposes it. */
function firstWeekday(): number {
  try {
    const locale = new Intl.Locale(navigator.language) as Intl.Locale & {
      getWeekInfo?: () => { firstDay: number };
      weekInfo?: { firstDay: number };
    };
    return (locale.getWeekInfo?.() ?? locale.weekInfo)?.firstDay ?? 1;
  } catch {
    return 1;
  }
}

function calendarRange(preset: KibanaTimePreset, now: Date): [Date, Date] {
  const y = now.getFullYear();
  const m = now.getMonth();
  const d = now.getDate();
  switch (preset) {
    case 'today':
      return [new Date(y, m, d), new Date(y, m, d + 1)];
    case 'thisWeek': {
      const isoDay = now.getDay() === 0 ? 7 : now.getDay();
      const back = (isoDay - firstWeekday() + 7) % 7;
      return [new Date(y, m, d - back), new Date(y, m, d - back + 7)];
    }
    case 'thisMonth':
      return [new Date(y, m, 1), new Date(y, m + 1, 1)];
    default:
      return [new Date(y, 0, 1), new Date(y + 1, 0, 1)];
  }
}

/** One absolute range for a selection; hits and histogram of a run both send this exact range. */
export function resolveTime(selection: KibanaTimeSelection, now = new Date()): KibanaTimeRange {
  if (selection.mode === 'absolute') return { from: selection.from, to: selection.to };
  const span = RELATIVE_MS[selection.preset];
  if (span !== undefined) {
    return { from: new Date(now.getTime() - span).toISOString(), to: now.toISOString() };
  }
  const [from, to] = calendarRange(selection.preset, now);
  return { from: from.toISOString(), to: new Date(to.getTime() - 1).toISOString() };
}
