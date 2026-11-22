import type { ApiRequestPayload, KubeReadPayload } from '@shared/domain/dbmcp';
import type { KeptStaged } from '@shared/domain/kept';
import { MASKED_TEXT } from '@theme/format';
import { type KeptRange, keptRefs } from '../kept/keptHighlight';

// What a prepare script set, as the request box lists it: one row per changed key, with the value the
// request now carries. The same keys mark their tokens in the request text (`data-prepared`).
export interface PreparedValue {
  key: string;
  value: string;
  /** A saved secret the script wrote: shows a Secret badge, value masked. */
  secret?: boolean;
}

/** Underline for a token in the request text; the box mark for a body. Both read as "set by the script". */
export const PREPARED_MARK = 'underline decoration-dotted decoration-warn underline-offset-4';
export const PREPARED_BOX = 'border-l-2 border-dotted border-warn pl-2';

const BODY_PREVIEW = 80;
const quote = (s: string): string => (s === '' ? '""' : s);

/** The key a saved-variable write carries in the list and in `data-prepared`: `plain.<n>` or `secret.<n>`. */
export const writeKey = (w: Pick<KeptStaged, 'name' | 'secret'>): string =>
  `${w.secret ? 'secret' : 'plain'}.${w.name}`;

const writeRows = (writes: readonly KeptStaged[]): PreparedValue[] =>
  writes
    .filter((w) => !w.deleted)
    .map((w) => ({
      key: writeKey(w),
      value: w.secret ? MASKED_TEXT : quote(w.preview),
      secret: w.secret || undefined,
    }));

/** Ranges of `{{plain.n}}` and `{{secret.n}}` tokens in `text` that a prepare script wrote, each with its write key. */
export function writtenRanges(
  text: string,
  writes: readonly KeptStaged[],
): (KeptRange & { key: string })[] {
  if (!writes.length) return [];
  const keys = new Set(writes.filter((w) => !w.deleted).map(writeKey));
  return keptRefs(text).flatMap((r) => {
    const key = writeKey(r);
    return keys.has(key) ? [{ from: r.from, to: r.to, key }] : [];
  });
}

export function kubePrepared(
  payload: Pick<KubeReadPayload, 'request' | 'changed'> & Partial<Pick<KubeReadPayload, 'script'>>,
): PreparedValue[] {
  const req = payload.request;
  const changed = req
    ? payload.changed.map((key) => {
        const v = (req as Record<string, string | number | boolean>)[key];
        return { key, value: quote(String(v ?? '')) };
      })
    : [];
  return [...changed, ...writeRows(payload.script?.prepare?.writes ?? [])];
}

const pairsText = (pairs: readonly { name: string; value: string }[], sep: string): string =>
  pairs.map((p) => `${p.name}${sep}${p.value}`).join(', ');

export function apiPrepared(payload: ApiRequestPayload): PreparedValue[] {
  const body = payload.body;
  const value: Record<string, string> = {
    method: payload.method,
    url: payload.url,
    headers: pairsText(payload.headers, ': '),
    query: pairsText(payload.query, '='),
    form: pairsText(body?.form ?? [], '='),
    contentType: body?.contentType ?? '',
    body: (body?.text ?? '').slice(0, BODY_PREVIEW),
  };
  const changed = (payload.script?.prepare?.changed ?? []).map((key) => ({
    key,
    value: quote(value[key] ?? ''),
  }));
  return [...changed, ...writeRows(payload.script?.prepare?.writes ?? [])];
}
