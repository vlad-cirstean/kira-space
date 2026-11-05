import type {
  KibanaDataView,
  KibanaDiscoverRequest,
  KibanaField,
  KibanaLanguage,
} from '@shared/domain/kibana';
import { newUuid } from '@shared/ids';
import { t } from '@theme/i18n';
import { promiseTimeout } from '@vueuse/core';
import { matchesQuery } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { queryClient } from '@workbench/state/queryClient';
import { control } from '../../../bridge/control';
import type {
  EditorAsyncCompletionSource,
  EditorCompletion,
  EditorCompletionSource,
} from '../../../editor/completion';
import { type KqlContext, kqlCompletionContext } from './kqlContext';

export const KQL_TRIGGER_CHARACTERS = [':', ' ', '(', '"', '<', '>', '='] as const;

const FIRST_FETCH_SIZE = 100;
const SUGGEST_STALE_MS = 30_000;
const MAX_FIELD_OPTIONS = 500;
const RANGE_TYPES = new Set(['number', 'date', 'ip']);
const VALUE_TYPES = new Set(['string', 'number', 'boolean', 'ip']);
const NEEDS_QUOTES = /[\s():<>"{}\\*]/;

export interface KqlCompletionDeps {
  connectionId: () => string;
  dataView: () => KibanaDataView | null;
  language: () => KibanaLanguage;
  /** A size-0 request for `query` carrying the tab's time range, filters and zone. */
  request: (query: string) => KibanaDiscoverRequest;
  /** Changes whenever the time selection or filters do, so cached values never outlive them. */
  scopeKey: () => string;
}

function fieldByName(view: KibanaDataView | null, name: string): KibanaField | undefined {
  return view?.fields.find((f) => f.name === name);
}

function fieldText(name: string): string {
  return NEEDS_QUOTES.test(name) ? `"${name.replace(/(["\\])/g, '\\$1')}"` : name;
}

function startsWith(label: string, prefix: string): boolean {
  return label.toLowerCase().startsWith(prefix.toLowerCase());
}

function conjunctions(prefix: string): EditorCompletion[] {
  return (
    [
      ['and', t('studio.kibana.completion.and.description')],
      ['or', t('studio.kibana.completion.or.description')],
    ] as const
  )
    .filter(([label]) => startsWith(label, prefix))
    .map(([label, detail]) => ({ label, detail, type: 'keyword' as const }));
}

function operators(field: KibanaField): EditorCompletion[] {
  const out: EditorCompletion[] = [
    { label: ':', detail: t('studio.kibana.completion.equals.description'), type: 'keyword' },
    {
      label: ': *',
      detail: t('studio.kibana.completion.exists.description'),
      type: 'keyword',
    },
  ];
  if (RANGE_TYPES.has(field.type)) {
    out.push(
      { label: '>=', detail: t('studio.kibana.completion.atLeast.description'), type: 'keyword' },
      { label: '>', detail: t('studio.kibana.completion.above.description'), type: 'keyword' },
      { label: '<=', detail: t('studio.kibana.completion.atMost.description'), type: 'keyword' },
      { label: '<', detail: t('studio.kibana.completion.below.description'), type: 'keyword' },
    );
  }
  return out;
}

function fieldOptions(view: KibanaDataView | null, ctx: Extract<KqlContext, { kind: 'field' }>) {
  const out: EditorCompletion[] = [];
  const nestedPrefix = ctx.nested ? `${ctx.nested}.` : '';
  for (const f of view?.fields ?? []) {
    if (!f.searchable) continue;
    if (nestedPrefix && !f.name.startsWith(nestedPrefix)) continue;
    const shown = nestedPrefix ? f.name.slice(nestedPrefix.length) : f.name;
    if (!matchesQuery(shown, ctx.prefix)) continue;
    out.push({ label: shown, insert: fieldText(shown), detail: f.type, type: 'property' });
    if (out.length >= MAX_FIELD_OPTIONS) break;
  }
  if (!nestedPrefix && startsWith('not', ctx.prefix)) {
    out.push({
      label: 'not',
      detail: t('studio.kibana.completion.not.description'),
      type: 'keyword',
    });
  }
  return out;
}

/** Field names only: Lucene syntax has no grammar-aware completion. */
function luceneFieldSource(deps: KqlCompletionDeps): EditorCompletionSource {
  return ({ doc, offset }) => {
    const before = doc.slice(0, offset);
    const word = /[^\s()[\]{}:"+\-~^/]*$/.exec(before)?.[0] ?? '';
    const from = offset - word.length;
    // Past a `:` the word is a value, not a field.
    if (from > 0 && /[:]$/.test(before.slice(0, from))) return null;
    const options = (deps.dataView()?.fields ?? [])
      .filter((f) => f.searchable && matchesQuery(f.name, word))
      .slice(0, MAX_FIELD_OPTIONS)
      .map<EditorCompletion>((f) => ({
        label: f.name,
        insert: `${f.name}:`,
        detail: f.type,
        type: 'property',
      }));
    return options.length > 0 ? { from, options } : null;
  };
}

function kqlSyncSource(deps: KqlCompletionDeps): EditorCompletionSource {
  return ({ doc, offset }) => {
    const ctx = kqlCompletionContext(doc, offset);
    switch (ctx.kind) {
      case 'field': {
        const options = fieldOptions(deps.dataView(), ctx);
        return options.length > 0 ? { from: ctx.range.from, options } : null;
      }
      case 'operator': {
        const field = fieldByName(deps.dataView(), ctx.field);
        // A bare word that is no field is free text, so a connective follows.
        const options = field ? operators(field) : conjunctions('');
        return { from: ctx.range.from, options };
      }
      case 'conjunction': {
        const options = conjunctions(ctx.prefix);
        return options.length > 0 ? { from: ctx.range.from, options } : null;
      }
      default:
        return null;
    }
  };
}

function valueText(field: KibanaField, value: string, closeQuote: boolean, quoted: boolean) {
  if (field.type === 'number' || field.type === 'boolean') return value;
  const escaped = value.replace(/(["\\])/g, '\\$1');
  return quoted && !closeQuote ? `"${escaped}` : `"${escaped}"`;
}

function kqlValueSource(deps: KqlCompletionDeps): EditorAsyncCompletionSource {
  // Callers sharing one in-flight fetch per key; the backend op is cancelled when the last one aborts.
  const waiters = new Map<string, number>();

  async function fetchValues(
    field: string,
    query: string,
    prefix: string,
    size: number,
    signal: AbortSignal,
  ) {
    const view = deps.dataView();
    const connectionId = deps.connectionId();
    if (!view) return null;
    const queryKey = [
      'kibana',
      'suggest',
      connectionId,
      view.id,
      field,
      deps.scopeKey(),
      query,
      prefix,
    ] as const;
    const mapKey = JSON.stringify(queryKey);
    let released = false;
    const release = () => {
      if (released) return false;
      released = true;
      const left = (waiters.get(mapKey) ?? 1) - 1;
      if (left > 0) {
        waiters.set(mapKey, left);
        return false;
      }
      waiters.delete(mapKey);
      return true;
    };
    const onAbort = () => {
      if (release()) {
        fireAndForget(queryClient.cancelQueries({ queryKey, exact: true }), 'kibana.suggest');
      }
    };
    if (signal.aborted) return null;
    waiters.set(mapKey, (waiters.get(mapKey) ?? 0) + 1);
    signal.addEventListener('abort', onAbort, { once: true });
    return queryClient
      .query({
        queryKey,
        staleTime: SUGGEST_STALE_MS,
        // The query's own signal aborts only once every sharer is gone; a caller's signal would
        // cancel the op under later callers that joined the same in-flight fetch.
        queryFn: ({ signal: querySignal }) => {
          const opId = newUuid();
          const cancel = () => fireAndForget(control.opsCancel(opId), 'kibana.suggest');
          if (querySignal.aborted) return Promise.reject(querySignal.reason);
          querySignal.addEventListener('abort', cancel, { once: true });
          return control
            .kibanaSuggest({
              connectionId,
              opId,
              tabId: '',
              request: { ...deps.request(query), size },
              field,
              prefix,
            })
            .finally(() => querySignal.removeEventListener('abort', cancel));
        },
      })
      .finally(() => {
        signal.removeEventListener('abort', onAbort);
        release();
      });
  }

  return async ({ doc, offset }, signal) => {
    const ctx = kqlCompletionContext(doc, offset);
    if (ctx.kind !== 'value' || deps.language() !== 'kuery') return null;
    const field = fieldByName(deps.dataView(), ctx.field);
    if (!field || !VALUE_TYPES.has(field.type)) return null;
    try {
      // The unfiltered list is fetched once per field and context; a complete one filters locally.
      const base = await fetchValues(
        ctx.field,
        ctx.queryWithoutValue,
        '',
        FIRST_FETCH_SIZE,
        signal,
      );
      if (!base || signal.aborted) return null;
      let values = base.values;
      let incomplete = false;
      if (ctx.prefix !== '') {
        if (base.complete) {
          values = values.filter((v) => matchesQuery(v, ctx.prefix));
        } else {
          await promiseTimeout(TIMING.search);
          if (signal.aborted) return null;
          const narrowed = await fetchValues(
            ctx.field,
            ctx.queryWithoutValue,
            ctx.prefix,
            FIRST_FETCH_SIZE,
            signal,
          );
          if (!narrowed || signal.aborted) return null;
          values = narrowed.values;
          incomplete = !narrowed.complete;
        }
      } else {
        incomplete = !base.complete;
      }
      const closeQuote = doc.charAt(offset) !== '"';
      const options = values.map<EditorCompletion>((v) => ({
        label: v,
        insert: valueText(field, v, closeQuote, ctx.quoted),
        detail: t('studio.kibana.completion.value.description'),
        type: 'variable',
      }));
      return options.length > 0 ? { from: ctx.range.from, options, incomplete } : null;
    } catch {
      // Value hints are best-effort; a failed or cancelled lookup leaves the list empty.
      return null;
    }
  };
}

export function createKqlCompletion(deps: KqlCompletionDeps): {
  completionSources: EditorCompletionSource[];
  asyncCompletionSources: EditorAsyncCompletionSource[];
} {
  const kql = kqlSyncSource(deps);
  const lucene = luceneFieldSource(deps);
  return {
    completionSources: [(ctx) => (deps.language() === 'kuery' ? kql(ctx) : lucene(ctx))],
    asyncCompletionSources: [kqlValueSource(deps)],
  };
}
