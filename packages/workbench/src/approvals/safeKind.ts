import { logWarn } from '../bridge/log';

export interface Safe<T> {
  value: T;
  failed: boolean;
}

// Kind code parses a payload it does not control; one throw must not blank the card.
export function safeKind<T>(kind: string, fn: () => T, fallback: T): Safe<T> {
  try {
    return { value: fn(), failed: false };
  } catch (err) {
    logWarn('approvals', `kind ${kind} unreadable`, err);
    return { value: fallback, failed: true };
  }
}
