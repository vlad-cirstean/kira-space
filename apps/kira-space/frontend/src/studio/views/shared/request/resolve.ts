import { type Reference, resolve, sanitizeUrlSpan } from '@kira/api-core';

// httprequest/state.ts's resolveTabState and grpcrequest/state.ts's
// resolveGrpcTabState each hand-rolled the same refs accumulator + sub closure over resolve(),
// and the same enabled-and-named pair filter/map (headers vs metadata). HTTP additionally needs
// a URL-safe variant (subUrl) neither request kind's other substitutable fields do.

export interface Substituter {
  /** Substitutes {{name}} references in free text (a header value, a gRPC message, …). */
  sub(text: string): string;
  /** Same, with delimiter-safe sanitization for a query-string context — HTTP's URL field only. */
  subUrl(text: string): string;
  /** Every reference substitution made through `sub`/`subUrl` so far. */
  refs: Reference[];
}

export function createSubstituter(
  values: Readonly<Record<string, string>>,
  secretNames: readonly string[],
  dynamic?: (name: string) => string | null,
): Substituter {
  const refs: Reference[] = [];
  const sub = (text: string): string => {
    const result = resolve(text, values, secretNames, dynamic);
    refs.push(...result.refs);
    return result.text;
  };
  const subUrl = (text: string): string => {
    const result = resolve(text, values, secretNames, dynamic, sanitizeUrlSpan);
    refs.push(...result.refs);
    return result.text;
  };
  return { sub, subUrl, refs };
}

/** The header/metadata row shape both request kinds filter and substitute identically: only
 *  enabled, named rows cross the wire. */
export function resolvePairs<T extends { enabled: boolean; name: string; value: string }>(
  list: readonly T[],
  sub: (text: string) => string,
): { name: string; value: string }[] {
  return list
    .filter((p) => p.enabled && p.name.trim() !== '')
    .map((p) => ({ name: sub(p.name), value: sub(p.value) }));
}
