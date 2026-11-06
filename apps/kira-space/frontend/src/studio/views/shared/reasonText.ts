import { t } from '@theme/i18n';
import type { CatalogReason } from './document/rawTree';

/** Resolves a worker-safe `{ key, params }` failure reason on the main thread. */
export function reasonText(reason: CatalogReason): string {
  return t(reason.key, reason.params);
}
