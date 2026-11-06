<script setup lang="ts">
import {
  type KibanaHistogram,
  type KibanaInterval,
  kibanaIntervalSchema,
} from '@shared/domain/kibana';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Skeleton } from '@theme/components/ui/skeleton';
import { formatDateTime, formatWeekdayDate } from '@theme/format';
import { type MessageKey, t } from '@theme/i18n';
import { formatCount } from '@theme/i18n/number';
import type { EChartsOption } from 'echarts';
import { computed, useTemplateRef } from 'vue';
import { VChart } from '../../../charts/echarts';
import { useChartTheme } from '../../../charts/theme';

const props = defineProps<{
  histogram: KibanaHistogram | null;
  loading: boolean;
  error: string | null;
}>();
const interval = defineModel<KibanaInterval>('interval', { required: true });
const emit = defineEmits<{ range: [from: string, to: string] }>();

const theme = useChartTheme();
const chart = useTemplateRef<InstanceType<typeof VChart>>('chart');

// Drag selects a time range without a toolbox button.
function enableBrush(): void {
  chart.value?.dispatchAction({
    type: 'takeGlobalCursor',
    key: 'brush',
    brushOption: { brushType: 'lineX', brushMode: 'single' },
  });
}

const INTERVAL_KEYS: Record<KibanaInterval, MessageKey> = {
  auto: 'studio.kibana.interval.auto.option',
  m: 'studio.kibana.interval.minute.option',
  h: 'studio.kibana.interval.hour.option',
  d: 'studio.kibana.interval.day.option',
  w: 'studio.kibana.interval.week.option',
  M: 'studio.kibana.interval.month.option',
  y: 'studio.kibana.interval.year.option',
};
const CHOSEN_VALUE: Partial<Record<KibanaInterval, string>> = {
  m: '1m',
  h: '1h',
  d: '1d',
  w: '1w',
  M: '1M',
  y: '1y',
};

const buckets = computed(() => props.histogram?.buckets ?? []);
const totalLabel = computed(() => {
  const h = props.histogram;
  if (!h) return '';
  const count = formatCount(h.total);
  return h.relation === 'gte'
    ? t('studio.kibana.histogram.totalAtLeast.label', { count })
    : t('studio.kibana.histogram.total.label', { count });
});
const scaled = computed(() => {
  const want = CHOSEN_VALUE[interval.value];
  return want !== undefined && props.histogram !== null && props.histogram.interval !== want;
});

// Buckets are cut in the browser's zone (the request's `timeZone`), so default formatting matches.
const subDay = computed(() => /(?:ms|[smh])$/.test(props.histogram?.interval ?? ''));
function axisLabel(key: number): string {
  return subDay.value ? formatDateTime(key) : formatWeekdayDate(key);
}

/** Exclusive upper edge of bucket `i`: the next key, else the previous bucket's width. */
function bucketEnd(i: number): number {
  const list = buckets.value;
  const next = list[i + 1];
  if (next) return next.key;
  const prev = list[i - 1];
  const cur = list[i];
  if (!cur) return 0;
  return cur.key + (prev ? cur.key - prev.key : 60_000);
}

const option = computed<EChartsOption>(() => {
  const c = theme.value;
  return {
    animation: false,
    grid: { left: 40, right: 12, top: 8, bottom: 24 },
    textStyle: { color: c.mutedText },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: c.tooltipBackground,
      borderColor: c.tooltipBorder,
      textStyle: { color: c.tooltipText },
      formatter: (params: unknown) => {
        const first = (params as { dataIndex: number; value: number }[])[0];
        const bucket = first ? buckets.value[first.dataIndex] : undefined;
        if (!first || !bucket) return '';
        return `${formatDateTime(bucket.key)} – ${formatDateTime(bucketEnd(first.dataIndex) - 1)}<br/><b>${formatCount(first.value)}</b>`;
      },
    },
    xAxis: {
      type: 'category',
      data: buckets.value.map((b) => b.key),
      axisLine: { lineStyle: { color: c.axisLine } },
      axisLabel: { color: c.mutedText, formatter: (v: string) => axisLabel(Number(v)) },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: c.mutedText },
      splitLine: { lineStyle: { color: c.splitLine } },
    },
    brush: {
      xAxisIndex: 0,
      brushType: 'lineX',
      brushMode: 'single',
      transformable: false,
      brushStyle: { color: c.brush, borderColor: c.bar },
    },
    toolbox: { show: false },
    series: [
      {
        type: 'bar',
        data: buckets.value.map((b) => b.count),
        itemStyle: { color: c.bar },
        emphasis: { itemStyle: { color: c.barHover } },
        barCategoryGap: '10%',
      },
    ],
  };
});

function toIso(ms: number): string {
  return new Date(ms).toISOString();
}

function onClick(e: { dataIndex?: number }): void {
  if (e.dataIndex === undefined) return;
  const bucket = buckets.value[e.dataIndex];
  if (bucket) emit('range', toIso(bucket.key), toIso(bucketEnd(e.dataIndex) - 1));
}

function onBrushEnd(e: unknown): void {
  const range = (e as { areas?: { coordRange?: [number, number] }[] }).areas?.[0]?.coordRange;
  if (!range) return;
  const lo = buckets.value[Math.max(0, Math.round(range[0]))];
  const hiIndex = Math.min(buckets.value.length - 1, Math.round(range[1]));
  if (lo && buckets.value[hiIndex]) emit('range', toIso(lo.key), toIso(bucketEnd(hiIndex) - 1));
}

function onInterval(value: unknown): void {
  const parsed = kibanaIntervalSchema.safeParse(value);
  if (parsed.success) interval.value = parsed.data;
}
</script>

<template>
  <section class="flex flex-col" data-testid="kibana-histogram">
    <header class="flex items-center gap-2 px-2 py-1 text-kira-sm text-muted-foreground">
      <span class="text-fg" data-testid="kibana-total">{{ totalLabel }}</span>
      <span v-if="scaled" data-testid="kibana-interval-scaled">{{
        t('studio.kibana.histogram.scaled.status', { interval: histogram?.interval ?? '' })
      }}</span>
      <span class="ml-auto" />
      <NativeSelect
        variant="bordered"
        :model-value="interval"
        :aria-label="t('studio.kibana.interval.select.aria')"
        data-testid="kibana-interval"
        @update:model-value="onInterval"
      >
        <option v-for="(key, value) in INTERVAL_KEYS" :key="value" :value="value">
          {{ t(key) }}
        </option>
      </NativeSelect>
    </header>
    <Alert v-if="error" strip variant="error" data-testid="kibana-histogram-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
    <div class="h-32 px-1">
      <Skeleton v-if="loading && !histogram" class="h-full w-full" />
      <VChart
        v-else
        ref="chart"
        class="h-full w-full"
        :option="option"
        autoresize
        @click="onClick"
        @brushEnd="onBrushEnd"
        @finished="enableBrush"
      />
    </div>
  </section>
</template>
