import type { ApiRequestPayload, KubeReadPayload } from '@shared/domain/dbmcp';

// What a prepare script set, as the request box lists it: one row per changed key, with the value the
// request now carries. The same keys mark their tokens in the request text (`data-prepared`).
export interface PreparedValue {
  key: string;
  value: string;
}

/** Underline for a token in the request text; the box mark for a body. Both read as "set by the script". */
export const PREPARED_MARK = 'underline decoration-dotted decoration-warn underline-offset-4';
export const PREPARED_BOX = 'border-l-2 border-dotted border-warn pl-2';

const BODY_PREVIEW = 80;
const quote = (s: string): string => (s === '' ? '""' : s);

export function kubePrepared(payload: KubeReadPayload): PreparedValue[] {
  const req = payload.request;
  if (!req) return [];
  return payload.changed.map((key) => {
    const v = (req as Record<string, string | number | boolean>)[key];
    return { key, value: quote(String(v ?? '')) };
  });
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
  return (payload.script?.prepare?.changed ?? []).map((key) => ({
    key,
    value: quote(value[key] ?? ''),
  }));
}
