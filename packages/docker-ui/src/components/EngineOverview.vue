<script setup lang="ts">
import { useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { type MessageKey, t } from '@theme/i18n';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { formatBytes, formatPercent } from '@workbench/behaviour/format';
import { computed, ref } from 'vue';
import {
  useContainerAction,
  useContainers,
  useDockerStatus,
  useImages,
  useNetworks,
  useVolumes,
} from '../queries';
import { useDockerStatsStore } from '../state/dockerStats';
import { type DockerSection, useDockerUiStore } from '../state/dockerUi';
import type { DockerStatsSample } from '../wire';
import ContainerTable from './ContainerTable.vue';
import DetailSection from './DetailSection.vue';
import EndpointChip from './EndpointChip.vue';
import EngineDiskSection from './EngineDiskSection.vue';
import StatsSparkline from './StatsSparkline.vue';
import UsageBar from './UsageBar.vue';

const scrollEl = ref<HTMLElement | null>(null);
const ui = useDockerUiStore();
const qc = useQueryClient();
const status = useDockerStatus();
const containers = useContainers();
const images = useImages();
const volumes = useVolumes();
const networks = useNetworks();
const stats = useDockerStatsStore();
const actions = useContainerAction();

const engine = computed(() => status.data.value?.engine);

const runningIds = computed(
  () =>
    new Set((containers.data.value ?? []).filter((c) => c.state === 'running').map((c) => c.id)),
);

const totals = computed(() => {
  let cpu = 0;
  let mem = 0;
  for (const [id, s] of stats.latest) {
    if (!runningIds.value.has(id)) continue;
    cpu += s.cpuPercent;
    mem += s.memUsage;
  }
  return { cpu, mem };
});

// Per-container rings share one cadence, so aligning them on the newest sample sums them.
function series(pick: (s: DockerStatsSample) => number): number[] {
  const rings = [...stats.history]
    .filter(([id]) => runningIds.value.has(id))
    .map(([, ring]) => ring);
  const n = Math.max(0, ...rings.map((r) => r.length));
  const out = new Array<number>(n).fill(0);
  for (const ring of rings) {
    const offset = n - ring.length;
    ring.forEach((s, i) => {
      out[offset + i] += pick(s);
    });
  }
  return out;
}

const cpuSeries = computed(() => series((s) => s.cpuPercent));
const memSeries = computed(() => series((s) => s.memUsage));

const cpuCapacity = computed(() => (engine.value?.cpus ?? 1) * 100);
const cpuPercentOfCapacity = computed(() => (totals.value.cpu / cpuCapacity.value) * 100);
const memPercentOfTotal = computed(() => {
  const total = engine.value?.memTotal ?? 0;
  return total > 0 ? (totals.value.mem / total) * 100 : 0;
});

const split = computed(() => {
  const e = engine.value;
  const total = Math.max(1, (e?.running ?? 0) + (e?.paused ?? 0) + (e?.stopped ?? 0));
  return {
    running: ((e?.running ?? 0) / total) * 100,
    paused: ((e?.paused ?? 0) / total) * 100,
    stopped: ((e?.stopped ?? 0) / total) * 100,
  };
});

const links = computed<
  Array<{ section: DockerSection; label: MessageKey; icon: string; count: number }>
>(() => [
  {
    section: 'images',
    label: 'docker.section.images.label',
    icon: 'package',
    count: images.data.value?.length ?? engine.value?.images ?? 0,
  },
  {
    section: 'volumes',
    label: 'docker.section.volumes.label',
    icon: 'database',
    count: volumes.data.value?.length ?? 0,
  },
  {
    section: 'networks',
    label: 'docker.field.networks.label',
    icon: 'globe',
    count: networks.data.value?.length ?? 0,
  },
]);

const facts = computed<Array<[string, string]>>(() => {
  const e = engine.value;
  if (!e) return [];
  return [
    [
      t('docker.engine.version.label'),
      t('docker.engine.versionApi.label', { version: e.version, api: e.apiVersion }),
    ],
    [t('docker.engine.osArch.label'), `${e.operatingSystem || e.os} / ${e.arch}`],
    [t('docker.engine.kernel.label'), e.kernelVersion],
    [t('docker.edit.cpus.label'), String(e.cpus)],
    [t('common.module.memory.label'), formatBytes(e.memTotal)],
    [t('docker.engine.endpoint.label'), status.data.value?.endpoint.host ?? ''],
  ];
});

const stoppedIds = computed(() =>
  (containers.data.value ?? [])
    .filter((c) => c.state !== 'running' && c.state !== 'paused')
    .map((c) => c.id),
);
const inFlight = computed(() =>
  [...runningIds.value, ...stoppedIds.value].some((id) => actions.isBusy(id)),
);

const bulk = ref<'start' | 'stop' | undefined>(undefined);
const startLocked = computed(
  () => stoppedIds.value.length === 0 || (inFlight.value && bulk.value === 'stop'),
);
const stopLocked = computed(
  () => runningIds.value.size === 0 || (inFlight.value && bulk.value === 'start'),
);

function runAll(ids: Iterable<string>, action: 'start' | 'stop'): void {
  bulk.value = action;
  for (const id of ids) {
    actions.run(id, action).catch(() => {
      // Surfaced through `actions.error`.
    });
  }
}

function openSection(section: DockerSection): void {
  ui.setSection(section);
}

function refresh(): Promise<void> {
  return qc.invalidateQueries({ queryKey: ['docker'] });
}
</script>

<template>
  <div ref="scrollEl" class="h-full overflow-auto p-3" data-testid="docker-engine-overview">
    <div v-if="engine" class="flex flex-col gap-3">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 class="text-kira-lg font-medium">{{ t('docker.engine.title.title') }}</h2>
        <Badge variant="info">{{ engine.version }}</Badge>
        <span class="inline-flex items-center gap-1 text-kira-sm text-muted-foreground"
          ><EndpointChip /></span
        >
        <span class="ml-auto flex items-center gap-1">
          <Button
            size="kira"
            variant="toolbar"
            :disabled="startLocked"
            :pending="inFlight && bulk === 'start'"
            data-testid="docker-start-stopped"
            @click="runAll(stoppedIds, 'start')"
          >
            <CodiconIcon name="play" :size="12" class="text-ok" />
            {{ t('docker.engine.startStopped.label') }}
          </Button>
          <Button
            size="kira"
            variant="toolbar"
            :disabled="stopLocked"
            :pending="inFlight && bulk === 'stop'"
            data-testid="docker-stop-all"
            @click="runAll(runningIds, 'stop')"
          >
            <CodiconIcon name="debug-stop" :size="12" class="text-error" />
            {{ t('common.action.stopAll.label') }}
          </Button>
          <Button
            size="kira"
            variant="toolbar"
            data-testid="docker-overview-refresh"
            @click="refresh"
          >
            <CodiconIcon name="refresh" :size="12" />{{ t('common.action.refresh.label') }}
          </Button>
        </span>
      </div>

      <div class="@container shrink-0">
        <div class="grid grid-cols-1 gap-3 @xl:grid-cols-2 @4xl:grid-cols-4">
          <DetailSection :title="t('common.field.status.label')">
            <div class="flex items-baseline gap-1.5">
              <span class="text-kira-xl font-medium" data-testid="docker-engine-count">{{
                engine.running
              }}</span>
              <span class="text-muted-foreground">{{ t('common.state.running.status') }}</span>
            </div>
            <div class="mt-2 flex h-1.5 w-full overflow-hidden rounded-full bg-field">
              <div class="bg-ok" :style="{ width: `${split.running}%` }" />
              <div class="bg-warn" :style="{ width: `${split.paused}%` }" />
              <div class="bg-subtle" :style="{ width: `${split.stopped}%` }" />
            </div>
            <div class="mt-2 text-kira-sm text-muted-foreground" data-testid="docker-engine-counts">
              {{
                t('docker.engine.counts.label', { stopped: engine.stopped, paused: engine.paused })
              }}
            </div>
          </DetailSection>
          <DetailSection :title="t('docker.metric.cpu.label')">
            <div class="flex items-baseline gap-1.5">
              <span class="text-kira-xl font-medium" data-testid="docker-engine-cpu">{{
                formatPercent(totals.cpu)
              }}</span>
              <span class="text-muted-foreground">{{
                t('docker.engine.of.label', { total: `${engine.cpus * 100}%` })
              }}</span>
            </div>
            <div class="mt-2"><UsageBar :percent="cpuPercentOfCapacity" /></div>
            <div class="mt-2"
              ><StatsSparkline
                :values="cpuSeries"
                :max="cpuCapacity"
                height-class="h-[2rem]"
              /></div
            >
          </DetailSection>
          <DetailSection :title="t('common.module.memory.label')">
            <div class="flex items-baseline gap-1.5">
              <span class="text-kira-xl font-medium" data-testid="docker-engine-mem">{{
                formatBytes(totals.mem)
              }}</span>
              <span class="text-muted-foreground">{{
                t('docker.engine.of.label', { total: formatBytes(engine.memTotal) })
              }}</span>
            </div>
            <div class="mt-2"><UsageBar :percent="memPercentOfTotal" /></div>
            <div class="mt-2"
              ><StatsSparkline
                :values="memSeries"
                :max="engine.memTotal"
                height-class="h-[2rem]"
              /></div
            >
          </DetailSection>
          <DetailSection :title="t('docker.engine.resources.title')" flush>
            <button
              v-for="l in links"
              :key="l.section"
              type="button"
              :class="
                cn(
                  rowVariants({ layout: 'menu' }),
                  'h-control-lg w-full cursor-default gap-2 rounded-none px-3 text-left',
                )
              "
              :data-testid="`docker-overview-link-${l.section}`"
              @click="openSection(l.section)"
            >
              <CodiconIcon :name="l.icon" :size="13" class="text-muted-foreground" />
              <span class="flex-1">{{ t(l.label) }}</span>
              <span :class="rowMeta">{{ l.count }}</span>
              <CodiconIcon name="chevron-right" :size="12" class="text-muted-foreground" />
            </button>
          </DetailSection>
        </div>
      </div>

      <EngineDiskSection />

      <DetailSection :title="t('docker.section.containers.label')" flush class="shrink-0">
        <ContainerTable :scroll-el="scrollEl" />
      </DetailSection>

      <DetailSection :title="t('docker.engine.engine.title')" class="shrink-0">
        <KeyValueList>
          <template v-for="[ k, v ] in facts" :key="k">
            <dt>{{ k }}</dt>
            <dd class="break-all font-data">{{ v }}</dd>
          </template>
        </KeyValueList>
      </DetailSection>
    </div>
  </div>
</template>
