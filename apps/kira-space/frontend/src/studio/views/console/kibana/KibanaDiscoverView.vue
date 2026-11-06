<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { colorMarkClass } from '@theme/connColor';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { usePersistedState } from '@workbench/behaviour/persisted';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue';
import type { ConsoleTabRecord } from '../../../../state/tabDomain';
import { useConnectionsStore } from '../../../state/connections';
import EngineIcon from '../../../theme/EngineIcon.vue';
import StaleDataStrip from '../../shared/StaleDataStrip.vue';
import { useConnectionGate } from '../../shared/useConnectionGate';
import ConsoleResultGrid from '../ConsoleResultGrid.vue';
import ConsoleResultStrip from '../ConsoleResultStrip.vue';
import { setText, useConsoleViewStore } from '../state';
import DataViewPicker from './DataViewPicker.vue';
import DiscoverSavedControls from './DiscoverSavedControls.vue';
import FieldSidebar from './FieldSidebar.vue';
import FilterBar from './FilterBar.vue';
import HitsHistogram from './HitsHistogram.vue';
import QueryBar from './QueryBar.vue';
import SortControl from './SortControl.vue';
import TimePicker from './TimePicker.vue';
import { useDiscover } from './useDiscover';

// ConsoleTabView.vue dispatches here for a kibana connection; MainView keys it by tab.id.
const props = defineProps<{ tab: ConsoleTabRecord }>();
const consoleViewStore = useConsoleViewStore();
const connectionsStore = useConnectionsStore();
const tabRef = computed(() => props.tab);

const { needsReconnect, reconnecting, onReconnectAndLoad } = useConnectionGate(() => props.tab);
const connected = computed(() => !needsReconnect.value);
const d = useDiscover(tabRef, connected);

const railColor = computed(() => {
  const rec = connectionsStore.connectionRecord(props.tab.connectionId);
  return rec ? (rec.color ?? null) : undefined;
});
const rt = d.runtime;
const hasResults = computed(() => (rt.value?.results.length ?? 0) > 0);
const stale = computed(() => consoleViewStore.isStale(props.tab.id));

const connect = useAsyncAction(async () => {
  if (!needsReconnect.value) return;
  if (await onReconnectAndLoad()) return;
  throw new Error(t('studio.console.connectFailed.error'));
});

async function onRun(): Promise<void> {
  if (!d.canRun.value || connect.pending.value) return;
  if (await connect.run()) await d.run();
}

async function onStaleReconnect(): Promise<void> {
  if (await onReconnectAndLoad()) consoleViewStore.restamp(props.tab.id);
}

// A tab opened from the tree runs once as soon as its connection is up; a revisit keeps its results.
let autoRan = false;
watch(
  () => [connected.value, d.dataView.value?.id] as const,
  ([up, viewId]) => {
    if (autoRan || !up || !viewId || hasResults.value || !d.canRun.value) return;
    autoRan = true;
    void d.run();
  },
  { immediate: true },
);

const queryBar = useTemplateRef<{ insertText: (text: string) => void }>('queryBar');

function quoteField(name: string): string {
  return /[\s():<>"{}\\*]/.test(name) ? `"${name.replace(/(["\\])/g, '\\$1')}"` : name;
}
function onInsertField(name: string): void {
  queryBar.value?.insertText(`${quoteField(name)}: `);
}
function onExists(name: string): void {
  filters.value = [
    ...filters.value,
    { field: name, operator: 'exists', values: [], disabled: false },
  ];
}

function onToggleColumn(name: string): void {
  d.toggleColumn(name);
  void onRun();
}

function onHistogramRange(from: string, to: string): void {
  d.patch({ time: { mode: 'absolute', from, to } });
  void onRun();
}

// Filters, sort, sample size, interval and time apply on the next run; they never run by themselves
// except where the user's gesture is the run (a bucket click, Enter, the Run button).
const filters = computed({
  get: () => d.state.value.filters,
  set: (v) => {
    d.patch({ filters: v });
    void onRun();
  },
});
const sort = computed({
  get: () => d.state.value.sort,
  set: (v) => {
    d.patch({ sort: v });
    void onRun();
  },
});
const sampleSize = computed({
  get: () => d.state.value.sampleSize,
  set: (v) => {
    d.patch({ sampleSize: v });
    void onRun();
  },
});
const interval = computed({
  get: () => d.state.value.interval,
  set: (v) => d.patch({ interval: v }),
});
const language = computed({
  get: () => d.state.value.language,
  set: (v) => d.patch({ language: v }),
});
const time = computed({
  get: () => d.state.value.time,
  set: (v) => {
    d.patch({ time: v });
    void onRun();
  },
});
const query = computed({
  get: () => props.tab.state.text,
  set: (v) => setText(props.tab.id, v),
});

const storedWidth = usePersistedState('kira.kibana.fieldSidebarWidth', 240);
const sidebar = useLiveSize(
  () => storedWidth.value,
  (size) => {
    storedWidth.value = size;
  },
);

const histogramError = computed(() =>
  d.histogramQuery.error.value ? String(d.histogramQuery.error.value) : null,
);
const showHistogram = computed(
  () => !!d.timeField.value || (d.histogramQuery.data.value?.total ?? 0) > 0,
);

const rootEl = useTemplateRef<HTMLElement>('rootEl');
const savedControls = useTemplateRef<{ save: () => Promise<void> }>('savedControls');
const savedError = ref<string | null>(null);
useShortcut(rootEl, ['form.save'], () => void savedControls.value?.save());

let off: Array<() => void> = [];
onMounted(() => {
  off = [
    registerCommand('view.run', () => void onRun()),
    registerCommand('view.refresh', () => void onRun()),
  ];
});
onUnmounted(() => {
  for (const fn of off) fn();
});
</script>

<template>
  <div
    ref="rootEl"
    class="flex h-full min-h-0 flex-col"
    data-testid="kibana-discover"
    :data-path="tab.path"
  >
    <ViewToolbar>
      <span v-if="railColor !== undefined" :class="colorMarkClass('dot', railColor)" />
      <span class="flex size-4 shrink-0 items-center justify-center">
        <EngineIcon kind="kibana" :size="13" />
      </span>
      <DataViewPicker
        :model-value="d.state.value.dataViewId"
        :connection-id="tab.connectionId"
        :enabled="connected"
        :fallback-label="d.state.value.dataViewName"
        @select="d.selectDataView"
      />
      <DiscoverSavedControls
        ref="savedControls"
        :d="d"
        :connection-id="tab.connectionId ?? ''"
        @run="onRun"
        @error="(m) => (savedError = m)"
      />
    </ViewToolbar>
    <div :class="colorMarkClass('band', railColor)" />

    <QueryBar
      ref="queryBar"
      v-model:doc="query"
      v-model:language="language"
      :completion-sources="d.completion.completionSources"
      :async-completion-sources="d.completion.asyncCompletionSources"
      :running="d.running.value"
      :can-run="d.canRun.value"
      :starting="connect.pending.value"
      @run="onRun"
      @stop="d.stop"
      @save="savedControls?.save()"
    >
      <TimePicker v-model="time" :disabled="!d.timeField.value" @refresh="onRun" />
    </QueryBar>
    <FilterBar
      v-model="filters"
      :fields="d.fields.value"
      :suggest="d.suggestValues"
      :suggest-scope="d.suggestScope.value"
    />

    <Alert strip v-if="savedError" variant="error" data-testid="kibana-saved-error">
      <AlertDescription>{{ savedError }}</AlertDescription>
    </Alert>
    <Alert
      strip
      v-if="rt?.status === 'error' && rt.error"
      variant="error"
      data-testid="kibana-error"
    >
      <AlertDescription>{{ rt.error.message }}</AlertDescription>
    </Alert>
    <Alert strip v-if="connect.error.value" variant="error" data-testid="kibana-connect-error">
      <AlertDescription>{{ connect.error.value }}</AlertDescription>
    </Alert>

    <div class="flex min-h-0 flex-1">
      <aside
        class="shrink-0"
        :style="{ flex: `0 0 ${sidebar.size.value}px` }"
        data-testid="kibana-sidebar"
      >
        <FieldSidebar
          :tab-id="tab.id"
          :fields="d.fields.value"
          :columns="d.state.value.columns"
          :loading="d.dataViewQuery.isFetching.value"
          @insert="onInsertField"
          @exists="onExists"
          @toggle-column="onToggleColumn"
        />
      </aside>
      <ResizeHandle
        :label="t('studio.kibana.fields.resize.aria')"
        axis="x"
        line
        :min="160"
        :max="480"
        v-bind="sidebar.bind"
      />
      <main class="flex min-w-0 flex-1 flex-col">
        <HitsHistogram
          v-if="showHistogram"
          v-model:interval="interval"
          :histogram="d.histogramQuery.data.value ?? null"
          :loading="d.histogramQuery.isFetching.value"
          :error="histogramError"
          @range="onHistogramRange"
        />
        <SortControl
          v-model:sort="sort"
          v-model:sample-size="sampleSize"
          :fields="d.fields.value"
          :time-field="d.timeField.value"
        />
        <div
          v-if="rt && hasResults"
          class="flex min-h-0 flex-1 flex-col"
          data-testid="kibana-results"
        >
          <StaleDataStrip
            v-if="stale"
            testid="kibana-stale"
            reconnect-only
            :reconnecting="reconnecting"
            @reconnect="onStaleReconnect"
          />
          <ConsoleResultStrip :tab-id="tab.id" />
          <div class="min-h-0 flex-1">
            <ConsoleResultGrid
              v-if="rt.activeKey"
              :page-key="rt.activeKey"
              :tab-id="tab.id"
              :connection-id="tab.connectionId"
              :path="tab.path"
            />
          </div>
        </div>
        <div
          v-else
          class="flex flex-1 items-center justify-center gap-2 text-kira-md text-muted-foreground"
          data-testid="kibana-empty"
        >
          <CodiconIcon name="search" :size="13" />
          {{ t('studio.kibana.results.empty') }}
        </div>
      </main>
    </div>
  </div>
</template>
