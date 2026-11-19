import { t } from '@theme/i18n';
import { type AnsiSegment, parseAnsi } from './ansi';

// Tail size and line cap shared by every streaming log view (container logs, pod logs).

/** Lines a streaming log keeps; older lines drop off the top. */
export const LOG_MAX_LINES = 20_000;

/** Tail sizes a log view offers; -1 is every line. */
export const LOG_TAILS = [100, 1000, 5000, -1] as const;

export function tailLabel(n: number): string {
  return n === -1 ? t('common.state.all.option') : t('common.logs.tail.option', { count: n });
}

/** Segmented-control items for the tail picker; each testid is `<testidPrefix>-<n>`. */
export function tailItems(testidPrefix: string) {
  return LOG_TAILS.map((n) => ({
    value: String(n),
    label: tailLabel(n),
    testid: `${testidPrefix}-${n}`,
  }));
}

/** `lines` plus `next`, trimmed to the newest `max`. */
export function appendCapped<T>(
  lines: readonly T[],
  next: readonly T[],
  max: number = LOG_MAX_LINES,
): T[] {
  const all = lines.concat(next);
  return all.length > max ? all.slice(all.length - max) : all;
}

/** A line's text without its ANSI styling. */
export function plainText(line: string | readonly AnsiSegment[]): string {
  return (typeof line === 'string' ? parseAnsi(line) : line).map((seg) => seg.text).join('');
}
