<script setup lang="ts">
import DetailSection from '@theme/components/DetailSection.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import { t } from '@theme/i18n';
import { formatBytes, formatPercent } from '@workbench/behaviour/format';
import { computed } from 'vue';
import { useDockerStatsStore } from '../state/dockerStats';
import ContainerSizeSection from './ContainerSizeSection.vue';
import StatsSparkline from './StatsSparkline.vue';

const props = defineProps<{ containerId: string; running: boolean }>();

const stats = useDockerStatsStore();
const latest = computed(() => stats.latest.get(props.containerId));
const history = computed(() => stats.history.get(props.containerId) ?? []);
const cpu = computed(() => history.value.map((s) => s.cpuPercent));
const mem = computed(() => history.value.map((s) => s.memUsage));
</script>

<template>
  <div class="h-full overflow-auto p-3" data-testid="docker-stats">
    <div class="@container flex flex-col gap-3">
      <ContainerSizeSection :id="containerId" />
      <p v-if="!running" class="text-muted-foreground" data-testid="docker-stats-stopped">{{
        t('docker.stats.stopped.emptyHint')
      }}</p>
      <p v-else-if="!latest" class="text-muted-foreground" data-testid="docker-stats-waiting">{{
        t('docker.stats.waiting.loading')
      }}</p>
      <div v-else class="grid grid-cols-1 gap-3 @3xl:grid-cols-2">
        <DetailSection :title="t('docker.metric.cpu.label')">
          <div class="flex items-baseline justify-between">
            <span class="text-kira-lg font-medium" data-testid="docker-stats-cpu">{{
              formatPercent(latest.cpuPercent)
            }}</span>
            <span class="text-kira-sm text-muted-foreground">{{
              t('docker.stats.samples.label', { count: cpu.length })
            }}</span>
          </div>
          <StatsSparkline :values="cpu" height-class="h-24" />
        </DetailSection>
        <DetailSection :title="t('common.module.memory.label')">
          <div class="flex items-baseline justify-between">
            <span class="text-kira-lg font-medium" data-testid="docker-stats-mem">{{
              formatBytes(latest.memUsage)
            }}</span>
            <span class="text-kira-sm text-muted-foreground">{{
              t('docker.stats.memOf.label', {
                limit: formatBytes(latest.memLimit),
                percent: formatPercent(latest.memPercent),
              })
            }}</span>
          </div>
          <StatsSparkline :values="mem" :max="latest.memLimit" height-class="h-24" />
        </DetailSection>
        <DetailSection :title="t('docker.stats.io.title')" class="@3xl:col-span-2">
          <KeyValueList>
            <dt>{{ t('docker.stats.network.label') }}</dt>
            <dd class="font-data"
              >{{ formatBytes(latest.netRx) }}
              / {{ formatBytes(latest.netTx) }}</dd
            >
            <dt>{{ t('docker.stats.block.label') }}</dt>
            <dd class="font-data"
              >{{ formatBytes(latest.blockRead) }}
              / {{ formatBytes(latest.blockWrite) }}</dd
            >
            <dt>{{ t('docker.stats.pids.label') }}</dt>
            <dd class="font-data">{{ latest.pids }}</dd>
          </KeyValueList>
        </DetailSection>
      </div>
    </div>
  </div>
</template>
