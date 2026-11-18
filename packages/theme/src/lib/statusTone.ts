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
// Issue status category (new, indeterminate, done) as the tracker reports it.
export const ISSUE_CATEGORY_TONE: Readonly<Record<string, StatusTone>> = {
  new: 'neutral',
  indeterminate: 'info',
  done: 'ok',
};
export const issueTone = (category?: string): StatusTone =>
  (category && ISSUE_CATEGORY_TONE[category]) || 'neutral';
// CI check state, keyed lowercase.
export const CHECK_TONE: Readonly<Record<string, StatusTone>> = {
  success: 'ok',
  failure: 'error',
  error: 'error',
  pending: 'neutral',
  expected: 'neutral',
  skipped: 'neutral',
  neutral: 'neutral',
};
export const checkTone = (state: string): StatusTone =>
  CHECK_TONE[state.toLowerCase()] ?? 'neutral';
// Added, changed, removed: one tone for every diff and change-kind surface.
export type ChangeKind = 'added' | 'changed' | 'removed' | 'unchanged';
export const CHANGE_TONE: Readonly<Record<ChangeKind, StatusTone>> = {
  added: 'ok',
  changed: 'warn',
  removed: 'error',
  unchanged: 'neutral',
};
