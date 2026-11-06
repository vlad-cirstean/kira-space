<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import {
  type ProofChartPayload,
  type ProofDiagramPayload,
  type ProofStepDetail,
  proofChartPayloadSchema,
  proofDiagramPayloadSchema,
} from '@shared/domain/proof';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import MonacoHost from '../editor/MonacoHost.vue';
import { useCellSelectionStore } from '../state/cellSelection';
import ConsoleResultGrid from '../views/console/ConsoleResultGrid.vue';
import CellEditorDock from '../views/shared/celleditor/CellEditorDock.vue';
import ProofChart from './ProofChart.vue';
import ProofDiagram from './ProofDiagram.vue';
import { buildProofPage, mountProofPage, proofPageKey, releaseProofPage } from './pages';
import { useProofStep } from './queries';
import { useProofWindowStore } from './state';
import { useStepViewState } from './useStepViewState';
import { proofViewIcon } from './viewIcon';

// One step: its description, the query that produced it (read-only), and the result in the same
// grid the query console uses, or a chart.
const props = defineProps<{ sessionId: string; rev: number; ordinal: number }>();

const store = useProofWindowStore();
const cellSelection = useCellSelectionStore();
const key = proofPageKey(props.sessionId, props.rev, props.ordinal);
const step = useProofStep(
  () => props.sessionId,
  () => props.rev,
  () => props.ordinal,
);
const view = useStepViewState({
  sessionId: () => props.sessionId,
  rev: () => props.rev,
  ordinal: () => props.ordinal,
});

const QUERY_MAX_HEIGHT = 240;
const LANGUAGES: Record<ProofStepDetail['language'], EditorLanguageId> = {
  sql: 'sql',
  json: 'json',
  shell: 'plain',
  text: 'plain',
};
const query = computed(() => step.data.value?.query ?? '');
const language = computed(() => LANGUAGES[step.data.value?.language ?? 'text']);
const queryHeight = ref(QUERY_MAX_HEIGHT);
const queryReady = ref(false);
const resultReady = ref(false);
// A capture waits for the cell editor too, but only while a cell is selected.
const dockReady = ref(false);
const hasCell = computed(() => cellSelection.selectedCellFor(key) !== null);
watch(hasCell, (open) => {
  if (open) dockReady.value = false;
});

const pageError = shallowRef<string | null>(null);
const pageMounted = ref(false);
const chartPayload = shallowRef<ProofChartPayload | null>(null);
const diagramPayload = shallowRef<ProofDiagramPayload | null>(null);
watch(
  () => step.data.value,
  (data) => {
    if (!data || pageMounted.value || chartPayload.value || diagramPayload.value) return;
    try {
      if (data.view === 'chart') {
        chartPayload.value = proofChartPayloadSchema.parse(data.data);
      } else if (data.view === 'diagram') {
        diagramPayload.value = proofDiagramPayloadSchema.parse(data.data);
      } else {
        mountProofPage(key, buildProofPage(data.view, data.data));
        pageMounted.value = true;
      }
    } catch (err) {
      pageError.value = errorText(err);
      store.markReady(key);
    }
  },
  { immediate: true },
);

watch(
  () =>
    resultReady.value && (!query.value || queryReady.value) && (!hasCell.value || dockReady.value),
  (ok, prev) => {
    if (ok) store.markReady(key);
    else if (prev) store.unmarkReady(key);
  },
  { immediate: true },
);

// A step that failed to load renders its error state; that is what a capture shows, so it is ready.
watch(
  () => step.isError.value || view.loadFailed.value,
  (failed) => {
    if (failed) store.markReady(key);
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  store.unmarkReady(key);
  if (pageMounted.value) releaseProofPage(key);
});
</script>

<template>
  <div
    class="flex h-full min-h-0 flex-col"
    data-testid="proof-step"
    :data-step-key="key"
    :data-ready="store.ready[key] ? 'true' : 'false'"
  >
    <QueryState :query="step">
      <template #default="{ data }">
        <EmptyState
          v-if="pageError"
          fill
          icon="warning"
          :title="t('common.state.loadFailed.title')"
          :description="pageError"
        />
        <template v-else>
          <div
            class="flex h-control-lg shrink-0 items-center gap-2 px-3"
            data-testid="proof-step-header"
          >
            <CodiconIcon
              :name="proofViewIcon(data.view)"
              :size="16"
              class="text-muted-foreground"
            />
            <span
              class="min-w-0 truncate text-kira-md font-medium"
              data-testid="proof-step-description"
              >{{
                data.description
              }}</span
            >
            <Badge
              class="shrink-0"
              :aria-label="t('common.field.source.label')"
              data-testid="proof-step-source"
              >{{
                data.source
              }}</Badge
            >
          </div>
          <div
            v-if="data.query && view.seed.value"
            class="shrink-0 overflow-hidden"
            :style="{ height: `${queryHeight}px` }"
            :aria-label="t('studio.console.plan.root.query.label')"
            data-testid="proof-step-query"
          >
            <MonacoHost
              :doc="data.query"
              :language="language"
              readonly
              force-wrap
              :scroll-top="view.seed.value.queryScrollTop"
              @update:scroll-top="view.setQueryScroll"
              @update:content-height="(h) => (queryHeight = Math.min(QUERY_MAX_HEIGHT, h))"
              @ready="queryReady = true"
            />
          </div>
          <div class="relative min-h-0 flex-1" data-testid="proof-step-result">
            <ConsoleResultGrid
              v-if="pageMounted && view.seed.value"
              :page-key="key"
              :tab-id="key"
              :connection-id="null"
              path=""
              :view-state="view.seed.value"
              @update:view-state="view.setResult"
              @ready="resultReady = true"
            />
            <ProofChart
              v-else-if="chartPayload && view.seed.value"
              :payload="chartPayload"
              :description="data.description"
              :seed="view.seed.value.chart"
              @update:chart="view.setChart"
              @ready="resultReady = true"
            />
            <ProofDiagram
              v-else-if="diagramPayload && view.seed.value"
              :payload="diagramPayload"
              :description="data.description"
              :seed="{
                top: view.seed.value.resultScrollTop,
                left: view.seed.value.resultScrollLeft,
              }"
              @update:scroll="view.setDiagramScroll"
              @ready="resultReady = true"
            />
          </div>
          <CellEditorDock :tab-id="key" readonly @ready="dockReady = true" />
        </template>
      </template>
    </QueryState>
  </div>
</template>
