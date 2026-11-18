<script setup lang="ts">
import DetailSection from '@theme/components/DetailSection.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@theme/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import {
  displayValue,
  formatBytes,
  formatDateTime,
  useRelativeTime,
} from '@workbench/behaviour/format';
import Identifier from '@workbench/components/Identifier.vue';
import { computed } from 'vue';
import { useEngineDiskUsage } from '../queries';

const VOLUME_LIMIT = 10;

const { query, refresh } = useEngineDiskUsage();

const usage = computed(() => query.data.value);
const measuring = computed(() => query.isFetching.value);
const takenAt = computed(() => (usage.value ? new Date(usage.value.takenAt) : new Date()));
const ago = useRelativeTime(takenAt);

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
  <DetailSection :title="t('docker.disk.title')" class="shrink-0">
    <template #actions>
      <span class="ml-auto flex items-center gap-1 normal-case tracking-normal">
        <Tooltip v-if="usage">
          <TooltipTrigger as-child>
            <span data-testid="docker-disk-taken">{{ ago }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ formatDateTime(usage.takenAt) }}</TooltipContent>
        </Tooltip>
        <TooltipIconButton
          icon="refresh"
          :label="t('docker.disk.measure.tooltip')"
          :disabled="measuring"
          data-testid="docker-disk-refresh"
          @click="refresh"
        />
      </span>
    </template>

    <Alert v-if="query.error.value" variant="error" data-testid="docker-disk-error">
      <AlertDescription>{{ query.error.value.message }}</AlertDescription>
    </Alert>

    <InlineState v-if="measuring && !usage" kind="loading">{{
      t('docker.measuring.loading')
    }}</InlineState>
    <div v-else-if="!usage" class="flex flex-wrap items-center gap-3">
      <InlineState kind="empty">{{ t('docker.disk.notMeasured.body') }}</InlineState>
      <Button size="kira" variant="toolbar" data-testid="docker-disk-measure" @click="refresh">{{
        t('docker.measure.label')
      }}</Button>
    </div>
    <template v-else>
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
              formatBytes(usage.total)
            }}</TableCell>
            <TableCell />
            <TableCell />
          </TableRow>
        </TableBody>
      </Table>
      <div v-if="topVolumes.length" class="mt-3">
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
  </DetailSection>
</template>
