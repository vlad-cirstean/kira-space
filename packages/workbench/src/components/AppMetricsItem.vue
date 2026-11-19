<script setup lang="ts">
import type { AppMetricsSample, AppMetricsUsage } from '@shared/protocol/events';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import { t } from '@theme/i18n';
import { formatBytes, formatPercent } from '@workbench/behaviour/format';
import StatusBarItem from '@workbench/components/StatusBarItem.vue';
import { computed } from 'vue';

// P116 H7: the status bar's CPU/memory readout, hoisted verbatim from Kira Studio's own
// StatusBar.vue (same data-testids) — a plain presentational item, the sample itself stays owned by
// each app's own appMetrics store (createAppMetricsStore.ts).
const props = defineProps<{ sample: AppMetricsSample | null; appName: string }>();

// Headline is the total across app, agent and script processes (internal/metrics/ticker.go's Interval, 5s);
// the tooltip splits it into App and Agents.
const cpuLabel = computed(() => (props.sample ? formatPercent(props.sample.cpuPercent) : ''));
const memLabel = computed(() => {
  const sample = props.sample;
  return sample ? formatBytes(sample.memoryBytes) : '';
});

const rows = computed(() =>
  props.sample
    ? [
        {
          label: t('workbench.statusBar.metrics.cores.label'),
          value: String(props.sample.logicalCPUs),
        },
      ]
    : [],
);

// App, Agents, Scripts, Total: the three groups are disjoint, the top-level sample figures are their sum.
const breakdown = computed(() => {
  const sample = props.sample;
  if (!sample) return [];
  const line = (group: string, label: string, u: AppMetricsUsage) => ({
    group,
    label,
    cpu: formatPercent(u.cpuPercent),
    memory: formatBytes(u.memoryBytes),
    processes: String(u.processCount),
  });
  return [
    line('app', t('common.module.app.label'), sample.app),
    line('agents', t('common.module.agents.label'), sample.agents),
    line('scripts', t('common.module.scripts.label'), sample.scripts),
    line('total', t('workbench.statusBar.metrics.total.label'), sample),
  ];
});
</script>

<template>
  <StatusBarItem v-if="sample" icon="pulse" as="span" data-testid="app-metrics">
    <!-- "100%", "12.3%" -->
    <span class="inline-block text-right min-w-[5ch]" data-testid="app-metrics-cpu">{{
      cpuLabel
    }}</span>
    <span class="text-subtle">·</span>
    <!-- "1234.5 MB" -->
    <span class="inline-block text-right min-w-[9ch]" data-testid="app-metrics-mem">{{
      memLabel
    }}</span>
    <template #tip>
      <TooltipDetails
        :title="t('workbench.statusBar.metrics.tooltip', { app: appName })"
        :rows="rows"
        :hint="t('workbench.statusBar.metrics.interval.hint')"
      >
        <div
          class="grid grid-cols-[auto_repeat(3,auto)] gap-x-3 gap-y-0.5 tabular-nums"
          data-testid="app-metrics-breakdown"
        >
          <div class="contents text-muted-foreground">
            <span />
            <span class="text-right">{{ t('common.metric.cpu.label') }}</span>
            <span class="text-right">{{ t('common.module.memory.label') }}</span>
            <span class="text-right">{{ t('workbench.statusBar.metrics.processes.label') }}</span>
          </div>
          <div
            v-for="row in breakdown"
            :key="row.group"
            class="contents"
            :class="{ 'font-medium': row.group === 'total' }"
            data-testid="app-metrics-row"
            :data-group="row.group"
          >
            <span>{{ row.label }}</span>
            <span class="text-right" data-col="cpu">{{ row.cpu }}</span>
            <span class="text-right" data-col="memory">{{ row.memory }}</span>
            <span class="text-right" data-col="processes">{{ row.processes }}</span>
          </div>
        </div>
      </TooltipDetails>
    </template>
  </StatusBarItem>
</template>
