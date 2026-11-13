import { t } from '@theme/i18n';
import { formatTimeAgo, type UseTimeAgoOptions, useTimeAgo } from '@vueuse/core';
import type { MaybeRefOrGetter } from 'vue';

// Formatting lives in `@theme/format`; this module re-exports it and keeps the reactive helper.
export {
  displayValue,
  formatAbsoluteUtc,
  formatAgeCompact,
  formatBytes,
  formatClockTime,
  formatCompactDuration,
  formatCount,
  formatCurrency,
  formatDateTime,
  formatDuration,
  formatElapsed,
  formatPercent,
  formatRelative,
  formatWeekdayDate,
  formatZonedWeekdayClock,
  localTimezone,
  monthNames,
  weekdayNames,
} from '@theme/format';

// `justNow` is a plain string in VueUse, so the options resolve per call, not at module load.
function relativeOptions(): UseTimeAgoOptions<false, 'minute' | 'hour' | 'day'> {
  return {
    messages: {
      justNow: t('format.relative.justNow'),
      past: (n: string) => t('format.relative.past', { time: n }),
      future: (n: string) => t('format.relative.future', { time: n }),
      invalid: '',
      minute: (n: number) => t('format.relative.unitMinute', { count: n }),
      hour: (n: number) => t('format.relative.unitHour', { count: n }),
      day: (n: number) => t('format.relative.unitDay', { count: n }),
    },
    units: [
      { max: 3_600_000, value: 60_000, name: 'minute' },
      { max: 86_400_000, value: 3_600_000, name: 'hour' },
      { max: Number.POSITIVE_INFINITY, value: 86_400_000, name: 'day' },
    ],
  };
}

/** `useRelativeTime`'s words for `at` as of `now`, for rows that share one clock instead of one timer each. */
export function formatRelativeAt(at: Date | number | string, now: Date | number): string {
  return formatTimeAgo(new Date(at), relativeOptions(), now);
}

/** A reactive `formatRelative`: the same words, kept current as time passes. */
export function useRelativeTime(at: MaybeRefOrGetter<Date | number | string>) {
  return useTimeAgo(at, relativeOptions());
}
