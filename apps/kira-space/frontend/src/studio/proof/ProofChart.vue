<script setup lang="ts">
import type { ProofChartPayload, ProofChartViewState } from '@shared/domain/proof';
import { t } from '@theme/i18n';
import { computed, shallowRef, useTemplateRef } from 'vue';
import { VChart } from '../charts/echarts';
import { useChartTheme } from '../charts/theme';
import { chartOption } from './chartOption';

// One chart step. The legend, the clicked point and the zoom window live in `state`; each change
// is emitted for ProofStepView to persist, and the option is built from it so a restore matches.
const props = defineProps<{
  payload: ProofChartPayload;
  description: string;
  seed: ProofChartViewState | undefined;
}>();
const emit = defineEmits<{ 'update:chart': [state: ProofChartViewState]; ready: [] }>();

const theme = useChartTheme();
const chart = useTemplateRef<InstanceType<typeof VChart>>('chart');
const state = shallowRef<ProofChartViewState>(
  props.seed ?? { hidden: [], selected: null, zoom: null },
);
// Not reactive: a wheel zoom must not rebuild the option under the user's fingers.
let zoom = state.value.zoom;

const option = computed(() => chartOption(props.payload, theme.value, { ...state.value, zoom }));

function commit(next: ProofChartViewState): void {
  state.value = next;
  emit('update:chart', next);
}

function onLegend(e: unknown): void {
  const { selected } = e as { selected: Record<string, boolean> };
  const hidden = Object.entries(selected)
    .filter(([, shown]) => !shown)
    .map(([name]) => name);
  commit({ ...state.value, hidden });
}

function onClick(e: { componentType?: string; seriesIndex?: number; dataIndex?: number }): void {
  if (e.componentType !== 'series' || e.seriesIndex === undefined || e.dataIndex === undefined) {
    return;
  }
  const cur = state.value.selected;
  const same = cur?.series === e.seriesIndex && cur.index === e.dataIndex;
  commit({
    ...state.value,
    selected: same ? null : { series: e.seriesIndex, index: e.dataIndex },
  });
}

const round2 = (n: number) => Math.round(n * 100) / 100;

function onZoom(): void {
  const opt = chart.value?.getOption() as
    | { dataZoom?: { start: number; end: number }[] }
    | undefined;
  const dz = opt?.dataZoom?.[0];
  if (!dz) return;
  const start = round2(dz.start);
  const end = round2(dz.end);
  zoom = start <= 0 && end >= 100 ? null : { start, end };
  commit({ ...state.value, zoom });
}

let announced = false;
function onFinished(): void {
  if (announced) return;
  announced = true;
  emit('ready');
}

const selectedAttr = computed(() => {
  const s = state.value.selected;
  return s ? `${s.series}:${s.index}` : '';
});
const zoomAttr = computed(() => {
  const z = state.value.zoom;
  return z ? `${z.start}-${z.end}` : '';
});
</script>

<template>
  <figure
    class="m-0 size-full min-h-0"
    :aria-label="t('studio.proof.chart.region.aria', { description })"
    data-testid="proof-chart"
    :data-chart-type="payload.type"
    :data-hidden="JSON.stringify(state.hidden)"
    :data-selected="selectedAttr"
    :data-zoom="zoomAttr"
  >
    <VChart
      ref="chart"
      class="size-full"
      :option="option"
      :update-options="{ notMerge: true }"
      autoresize
      @legendselectchanged="onLegend"
      @click="onClick"
      @datazoom="onZoom"
      @finished="onFinished"
    />
  </figure>
</template>
