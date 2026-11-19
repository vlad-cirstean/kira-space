import { relativeTimeOptions } from '@theme/format';
import { useTimeAgo } from '@vueuse/core';
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

/** A reactive `formatRelative`: the same words, kept current as time passes. */
export function useRelativeTime(at: MaybeRefOrGetter<Date | number | string>) {
  return useTimeAgo(at, relativeTimeOptions());
}
