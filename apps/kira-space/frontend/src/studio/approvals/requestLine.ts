import { splitTemplateSpans, splitUrl } from '@kira/api-core';

// The request line split into the parts a reader tells apart. Offsets stay those of
// the raw URL (no `new URL()`, no decoding) so kept-value ranges still land.
export type UrlTokenKind =
  | 'scheme'
  | 'host'
  | 'port'
  | 'path'
  | 'var'
  | 'sep'
  | 'key'
  | 'value'
  | 'hash';

export interface UrlToken {
  kind: UrlTokenKind;
  text: string;
  from: number;
}

export interface QueryPairTokens {
  lead: UrlToken[];
  value: UrlToken[];
  valueText: string;
}

export interface RequestLineTokens {
  origin: UrlToken[];
  segments: UrlToken[][];
  pairs: QueryPairTokens[];
  hash: UrlToken[];
}

// `{{name}}` references become `var` tokens inside any part.
function withVars(text: string, from: number, kind: UrlTokenKind): UrlToken[] {
  return splitTemplateSpans(text)
    .filter((s) => s.text !== '')
    .map((s) => ({ kind: s.isReference ? 'var' : kind, text: s.text, from: from + s.from }));
}

// Last ':' of the authority after any userinfo that is not inside a reference.
function portAt(authority: string): number {
  const at = authority.lastIndexOf(':');
  if (
    at < 0 ||
    at < authority.lastIndexOf('@') ||
    authority.indexOf('}}', at) >= 0 ||
    authority.indexOf(']', at) >= 0
  )
    return -1;
  return at;
}

export function tokenizeRequestUrl(url: string): RequestLineTokens {
  const { base, query, hash } = splitUrl(url);
  const scheme = base.indexOf('://');
  const authFrom = scheme < 0 ? 0 : scheme + 3;
  const slash = base.indexOf('/', authFrom);
  const authTo = slash < 0 ? base.length : slash;
  const authority = base.slice(authFrom, authTo);
  const port = portAt(authority);

  const origin: UrlToken[] = [];
  if (scheme >= 0) origin.push({ kind: 'scheme', text: base.slice(0, authFrom), from: 0 });
  const hostText = port < 0 ? authority : authority.slice(0, port);
  origin.push(...withVars(hostText, authFrom, 'host'));
  if (port >= 0) origin.push({ kind: 'port', text: authority.slice(port), from: authFrom + port });

  const segments: UrlToken[][] = [];
  const path = base.slice(authTo);
  let at = 0;
  while (at < path.length) {
    const next = path.indexOf('/', at + 1);
    const end = next < 0 ? path.length : next;
    segments.push(withVars(path.slice(at, end), authTo + at, 'path'));
    at = end;
  }

  const pairs: QueryPairTokens[] = [];
  let q = base.length + 1;
  if (url.charAt(base.length) === '?') {
    query.split('&').forEach((raw, i) => {
      const from = q;
      q += raw.length + 1;
      if (raw === '') return;
      const eq = raw.indexOf('=');
      const lead: UrlToken[] = [{ kind: 'sep', text: i === 0 ? '?' : '&', from: from - 1 }];
      const key = eq < 0 ? raw : raw.slice(0, eq);
      lead.push(...withVars(key, from, 'key'));
      if (eq >= 0) lead.push({ kind: 'sep', text: '=', from: from + eq });
      const valueText = eq < 0 ? '' : raw.slice(eq + 1);
      pairs.push({ lead, value: withVars(valueText, from + eq + 1, 'value'), valueText });
    });
  }

  const hashFrom = url.length - hash.length - 1;
  const hashTokens: UrlToken[] =
    url.charAt(hashFrom) === '#'
      ? [{ kind: 'hash', text: url.slice(hashFrom), from: hashFrom }]
      : [];
  return { origin, segments, pairs, hash: hashTokens };
}
