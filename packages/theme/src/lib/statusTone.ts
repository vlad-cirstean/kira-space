// RS.1 status tones. Badge, tone text and StatusDot read the same key.
export type StatusTone = 'ok' | 'warn' | 'error' | 'info' | 'neutral';
export type ToneBadgeVariant = 'ok' | 'warn' | 'error' | 'info' | 'default';
export const TONE_BADGE: Record<StatusTone, ToneBadgeVariant> = {
  ok: 'ok',
  warn: 'warn',
  error: 'error',
  info: 'info',
  neutral: 'default',
};
export const TONE_TEXT: Record<StatusTone, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  error: 'text-error',
  info: 'text-info',
  neutral: 'text-muted-foreground',
};
export const TONE_DOT: Record<StatusTone, 'ok' | 'warn' | 'error' | 'info' | 'muted'> = {
  ok: 'ok',
  warn: 'warn',
  error: 'error',
  info: 'info',
  neutral: 'muted',
};
// Keyed by string: git-ui and ADE carry the state as a plain string.
export const CHANGE_REQUEST_TONE: Readonly<Record<string, StatusTone>> = {
  open: 'info',
  draft: 'neutral',
  merged: 'ok',
  closed: 'neutral',
};
export const changeRequestTone = (state: string): StatusTone =>
  CHANGE_REQUEST_TONE[state] ?? 'neutral';
