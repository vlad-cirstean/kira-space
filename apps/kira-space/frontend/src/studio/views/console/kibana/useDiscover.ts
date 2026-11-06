import {
  type KibanaConsoleState,
  type KibanaDataView,
  type KibanaDiscoverRequest,
  type KibanaSavedLink,
  kibanaConsoleStateSchema,
  kibanaDiscoverRequestSchema,
} from '@shared/domain/kibana';
import type { SavedDiscoverQuery } from '@shared/domain/queries';
import { newUuid } from '@shared/ids';
import { useQuery } from '@tanstack/vue-query';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { KEEP_WHILE_OPEN, queryClient } from '@workbench/state/queryClient';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { computed, type Ref, ref, watch } from 'vue';
import type { ConsoleTabRecord } from '../../../../state/tabDomain';
import { control } from '../../../bridge/control';
import { setKibana, setText, useConsoleViewStore } from '../state';
import { createKqlCompletion } from './kqlCompletion';
import { discoverBody, discoverDirty, kibanaStateFromBody, savedLink } from './savedDiscover';
import { browserTimeZone, resolveTime } from './time';

const dataViewQueryKey = (connectionId: string, id: string) =>
  ['kibana', 'dataView', connectionId, id] as const;

const histogramKeyFor = (tabId: string) => ['kibana', 'histogram', tabId] as const;

// A tab keeps only its latest run's histogram: each run drops the earlier ones, tab close drops all.
registerTabRuntimeCleanup((tabId) =>
  queryClient.removeQueries({ queryKey: histogramKeyFor(tabId) }),
);

const SAMPLE_SIZES = [100, 500, 1000, 5000, 10000] as const;
const SUGGEST_LIMIT = 100;

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/** Discover state of one console tab and the requests it sends; the query text is the tab's own. */
export function useDiscover(tab: Ref<ConsoleTabRecord>, connected: Ref<boolean>) {
  const consoleViewStore = useConsoleViewStore();
  const connectionId = computed(() => tab.value.connectionId ?? '');

  const state = computed<KibanaConsoleState>(
    () => tab.value.state.kibana ?? kibanaConsoleStateSchema.parse({}),
  );

  function patch(change: Partial<KibanaConsoleState>): void {
    setKibana(tab.value.id, { ...state.value, ...change });
  }

  const dataViewQuery = useQuery(() => ({
    queryKey: dataViewQueryKey(connectionId.value, state.value.dataViewId ?? ''),
    queryFn: () => control.kibanaDataView(connectionId.value, state.value.dataViewId ?? ''),
    enabled: connected.value && !!connectionId.value && !!state.value.dataViewId,
    staleTime: 5 * 60_000,
  }));
  const dataView = computed<KibanaDataView | null>(() => dataViewQuery.data.value ?? null);
  const fields = computed(() => dataView.value?.fields ?? []);
  const timeField = computed(() => dataView.value?.timeFieldName ?? null);
  const timeZone = browserTimeZone();

  function buildRequest(query = tab.value.state.text): KibanaDiscoverRequest {
    const s = state.value;
    return {
      dataView: s.dataViewId ?? '',
      query,
      language: s.language,
      timeRange: timeField.value ? resolveTime(s.time) : null,
      filters: s.filters,
      sort: s.sort,
      size: s.sampleSize,
      timeZone,
      ...(s.columns.length > 0 ? { columns: s.columns } : {}),
    };
  }

  // Completion and suggestions send size-0 requests that never render a table.
  function searchRequest(query: string): KibanaDiscoverRequest {
    const { columns: _columns, ...rest } = buildRequest(query);
    return rest;
  }

  // The request of the latest run: hits and histogram both send exactly it, so they never disagree.
  const lastRequest = ref<KibanaDiscoverRequest | null>(null);
  let lastOwn = '';
  const runtime = computed(() => consoleViewStore.runtime[tab.value.id]);
  const running = computed(() => runtime.value?.status === 'running');
  const canRun = computed(() => !!state.value.dataViewId && !running.value);

  async function run(): Promise<void> {
    if (!canRun.value) return;
    const request = buildRequest();
    queryClient.removeQueries({ queryKey: histogramKeyFor(tab.value.id) });
    lastRequest.value = request;
    lastOwn = JSON.stringify(request);
    await consoleViewStore.run(tab.value.id, [lastOwn]);
  }

  function stop(): void {
    consoleViewStore.stop(tab.value.id);
  }

  const histogramQuery = useQuery(() => ({
    queryKey: [
      ...histogramKeyFor(tab.value.id),
      JSON.stringify(lastRequest.value),
      state.value.interval,
    ] as const,
    queryFn: ({ signal }: { signal: AbortSignal }) => {
      const opId = newUuid();
      signal.addEventListener(
        'abort',
        () => fireAndForget(control.opsCancel(opId), 'kibana.histogram'),
        { once: true },
      );
      return control.kibanaHistogram({
        connectionId: connectionId.value,
        opId,
        tabId: tab.value.id,
        request: lastRequest.value as KibanaDiscoverRequest,
        interval: state.value.interval,
      });
    },
    enabled: connected.value && lastRequest.value !== null,
    gcTime: KEEP_WHILE_OPEN,
  }));

  // `fresh` is a run started elsewhere; a remount re-applies the last run's request and keeps its histogram.
  function applyRequest(req: KibanaDiscoverRequest, fresh: boolean): void {
    setText(tab.value.id, req.query);
    patch({
      dataViewId: req.dataView,
      language: req.language,
      time: req.timeRange
        ? { mode: 'absolute', from: req.timeRange.from, to: req.timeRange.to }
        : state.value.time,
      filters: req.filters,
      sort: req.sort,
      columns: req.columns ?? [],
      sampleSize: SAMPLE_SIZES.find((s) => s === req.size) ?? state.value.sampleSize,
    });
    if (fresh) queryClient.removeQueries({ queryKey: histogramKeyFor(tab.value.id) });
    lastRequest.value = req;
  }

  // A run started elsewhere (Operations re-run) hydrates the controls from its statement.
  let hydrated = false;
  watch(
    () => runtime.value?.lastStatements,
    (statements) => {
      const fresh = hydrated;
      hydrated = true;
      const text = statements?.[0];
      if (!text || text === lastOwn) return;
      const parsed = kibanaDiscoverRequestSchema.safeParse(safeJson(text));
      if (parsed.success) {
        lastOwn = text;
        applyRequest(parsed.data, fresh);
      }
    },
    { immediate: true },
  );

  let rerunOnLoad = false;
  const body = computed(() => discoverBody(tab.value.state.text, state.value));
  const dirty = computed(() => discoverDirty(tab.value.state));

  /**
   * Restores a saved query exactly and links the tab to it. Returns whether the caller runs it
   * now; a different data view runs once its fields (and time field) arrive.
   */
  function applySaved(entry: KibanaSavedLink): boolean {
    const sameView = entry.body.dataViewId === state.value.dataViewId;
    setText(tab.value.id, entry.body.query);
    setKibana(tab.value.id, kibanaStateFromBody(entry.body, entry));
    lastOwn = '';
    rerunOnLoad = !sameView;
    return sameView;
  }
  function link(entry: SavedDiscoverQuery): void {
    patch({ saved: savedLink(entry) });
  }
  function unlink(): void {
    patch({ saved: null });
  }

  function toggleColumn(name: string): void {
    const cols = state.value.columns;
    patch({ columns: cols.includes(name) ? cols.filter((c) => c !== name) : [...cols, name] });
  }

  // A new data view drops sort and filters on fields it lacks, then reruns once its fields arrive.
  function selectDataView(view: { id: string; name: string }): void {
    if (view.id === state.value.dataViewId) return;
    patch({ dataViewId: view.id, dataViewName: view.name, sort: [], columns: [] });
    rerunOnLoad = true;
  }
  watch(dataView, (view) => {
    if (!view || view.id !== state.value.dataViewId) return;
    const names = new Set(view.fields.map((f) => f.name));
    const filters = state.value.filters.filter((f) => names.has(f.field));
    const sort = state.value.sort.filter((s) => names.has(s.field));
    const columns = state.value.columns.filter((c) => names.has(c));
    if (
      filters.length !== state.value.filters.length ||
      sort.length !== state.value.sort.length ||
      columns.length !== state.value.columns.length
    ) {
      patch({ filters, sort, columns });
    }
    if (rerunOnLoad) {
      rerunOnLoad = false;
      void run();
    }
  });

  const completion = createKqlCompletion({
    connectionId: () => connectionId.value,
    dataView: () => dataView.value,
    language: () => state.value.language,
    request: (query) => ({ ...searchRequest(query), size: 0 }),
    scopeKey: () => JSON.stringify([state.value.time, state.value.filters]),
  });

  // Names everything a suggestion depends on, so another connection or data view never shares one.
  const suggestScope = computed(
    () =>
      [
        connectionId.value,
        state.value.dataViewId ?? '',
        JSON.stringify([state.value.time, state.value.filters]),
      ] as const,
  );

  async function suggestValues(field: string): Promise<string[]> {
    const r = await control.kibanaSuggest({
      connectionId: connectionId.value,
      opId: '',
      tabId: tab.value.id,
      request: { ...searchRequest(''), size: SUGGEST_LIMIT },
      field,
      prefix: '',
    });
    return r.values;
  }

  return {
    state,
    patch,
    dataView,
    dataViewQuery,
    fields,
    timeField,
    timeZone,
    buildRequest,
    run,
    stop,
    canRun,
    running,
    runtime,
    histogramQuery,
    selectDataView,
    toggleColumn,
    body,
    dirty,
    applySaved,
    link,
    unlink,
    completion,
    suggestValues,
    suggestScope,
  };
}
