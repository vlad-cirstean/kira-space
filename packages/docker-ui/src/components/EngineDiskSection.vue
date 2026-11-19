<script setup lang="ts">
import SectionHeading from '@theme/components/SectionHeading.vue';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { displayValue, formatBytes } from '@workbench/behaviour/format';
import Identifier from '@workbench/components/Identifier.vue';
import { computed } from 'vue';
import { useEngineDiskUsage } from '../queries';
import MeasuredSection from './MeasuredSection.vue';

const VOLUME_LIMIT = 10;

const { query } = useEngineDiskUsage();

const usage = computed(() => query.data.value);

const categories = computed(() => {
  const u = usage.value;
  if (!u) return [];
  return [
    { key: 'images', label: t('docker.section.images.label'), c: u.images },
    { key: 'containers', label: t('docker.section.containers.label'), c: u.containers },
    { key: 'volumes', label: t('docker.section.volumes.label'), c: u.volumes },
    { key: 'build-cache', label: t('docker.disk.buildCache.label'), c: u.buildCache },
  ];
});

const topVolumes = computed(() => (usage.value?.volumeSizes ?? []).slice(0, VOLUME_LIMIT));
const moreVolumes = computed(() =>
  Math.max(0, (usage.value?.volumeSizes.length ?? 0) - VOLUME_LIMIT),
);
const sizeText = (n: number): string => (n < 0 ? displayValue(null) : formatBytes(n));
</script>

<template>
  <MeasuredSection
    :title="t('docker.disk.title')"
    class="shrink-0"
    testid="docker-disk"
    :query="query"
    :taken-at="(u) => u.takenAt"
    :measure-tooltip="t('docker.disk.measure.tooltip')"
    :not-measured="t('docker.disk.notMeasured.body')"
  >
    <template #default="{ data: disk }">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead />
            <TableHead class="text-right">{{ t('docker.field.size.label') }}</TableHead>
            <TableHead class="text-right">{{ t('docker.disk.countActive.label') }}</TableHead>
            <TableHead class="text-right">{{ t('docker.disk.reclaimable.label') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="r in categories" :key="r.key">
            <TableCell>{{ r.label }}</TableCell>
            <TableCell class="text-right font-data" :data-testid="`docker-disk-${r.key}`">{{
              formatBytes(r.c.size)
            }}</TableCell>
            <TableCell class="text-right font-data">{{ r.c.count }} ({{ r.c.active }})</TableCell>
            <TableCell class="text-right font-data">{{ formatBytes(r.c.reclaimable) }}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell class="font-medium">{{ t('docker.disk.usedByDocker.label') }}</TableCell>
            <TableCell class="text-right font-data font-medium" data-testid="docker-disk-total">{{
              formatBytes(disk.total)
            }}</TableCell>
            <TableCell />
            <TableCell />
          </TableRow>
        </TableBody>
      </Table>
      <div v-if="topVolumes.length">
        <SectionHeading :label="t('docker.disk.volumesBySize.label')" />
        <div
          v-for="v in topVolumes"
          :key="v.name"
          class="flex items-baseline gap-3"
          data-testid="docker-disk-volume"
          :data-name="v.name"
        >
          <Identifier class="flex-1" :value="v.name" />
          <span class="font-data">{{ sizeText(v.size) }}</span>
        </div>
        <div :class="rowMeta" v-if="moreVolumes" data-testid="docker-disk-more">{{
          t('docker.disk.more.label', { count: moreVolumes })
        }}</div>
      </div>
    </template>
  </MeasuredSection>
</template>
