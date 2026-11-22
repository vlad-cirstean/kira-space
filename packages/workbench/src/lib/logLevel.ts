export type LogLevel = 'error' | 'warn';

// Level words match in any case; klog's `E0522`/`W0522` prefixes only as written.
const ERROR_WORD = /\b(?:error|fatal|panic)\b|level=error/i;
const ERROR_KLOG = /\bE\d{4}\b/;
const WARN_WORD = /\b(?:warn|warning)\b|level=warn/i;
const WARN_KLOG = /\bW\d{4}\b/;

/** The severity a log line states, or null. Error wins when a line names both. */
export function logLevelOf(text: string): LogLevel | null {
  if (ERROR_WORD.test(text) || ERROR_KLOG.test(text)) return 'error';
  if (WARN_WORD.test(text) || WARN_KLOG.test(text)) return 'warn';
  return null;
}
