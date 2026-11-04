import type { CustomScript } from '@shared/domain/scripts';
import type { RefParamValue, RepoScriptRefValue } from './repoFormTypes';

export function refParamsToForm(params: RepoScriptRefValue['params']): Record<string, string[]> {
  return Object.fromEntries(
    Object.entries(params ?? {}).map(([k, v]) => [k, typeof v === 'string' ? [v] : v]),
  );
}

/** The file's params: the script's non-secret params that hold a value, one as a string. */
export function refParamsFromForm(
  values: Record<string, string[]>,
  script: CustomScript,
): Record<string, RefParamValue> | undefined {
  const out: Record<string, RefParamValue> = {};
  for (const p of script.params) {
    const v = (values[p.name] ?? []).filter((x) => x !== '');
    if (p.secret || v.length === 0) continue;
    out[p.name] = v.length === 1 ? v[0] : v;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

export const normalScripts = (records: readonly CustomScript[]): CustomScript[] =>
  records.filter((r) => r.kind === 'script');

export interface ScriptGroups {
  ungrouped: CustomScript[];
  groups: { collection: string; scripts: CustomScript[] }[];
}

/** Ungrouped scripts first, then one group per collection in first-seen order. */
export function groupScripts(scripts: readonly CustomScript[]): ScriptGroups {
  const ungrouped: CustomScript[] = [];
  const byName = new Map<string, CustomScript[]>();
  for (const s of scripts) {
    if (!s.collection) ungrouped.push(s);
    else byName.set(s.collection, [...(byName.get(s.collection) ?? []), s]);
  }
  return {
    ungrouped,
    groups: [...byName].map(([collection, list]) => ({ collection, scripts: list })),
  };
}

export type RefStatus =
  | { kind: 'ok'; script: CustomScript; unknown: string[]; secretGiven: string[] }
  | { kind: 'smart' | 'missing' }
  | { kind: 'duplicate'; count: number };

/** Mirrors the run-time refusals (ade.resolveScriptRef) so they show before a run fails. */
export function refStatus(records: readonly CustomScript[], ref: RepoScriptRefValue): RefStatus {
  const named = records.filter((r) => r.name === ref.script);
  const normal = named.filter((r) => r.kind === 'script');
  if (normal.length === 1) {
    const script = normal[0];
    const given = Object.keys(ref.params ?? {});
    return {
      kind: 'ok',
      script,
      unknown: given.filter((n) => !script.params.some((p) => p.name === n)),
      secretGiven: given.filter((n) => script.params.some((p) => p.name === n && p.secret)),
    };
  }
  if (normal.length > 1) return { kind: 'duplicate', count: normal.length };
  return { kind: named.length > 0 ? 'smart' : 'missing' };
}
