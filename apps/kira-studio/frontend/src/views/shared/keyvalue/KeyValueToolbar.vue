<script setup lang="ts">
import type { PageSize } from '@shared/domain/tabs';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { formatBytes, formatCompactDuration, formatCount } from '@workbench/behaviour/format';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, useTemplateRef } from 'vue';
import { pageSizeOptions } from '../page/sizes';
import KeyValueAddPopover from './KeyValueAddPopover.vue';
import KeyValueEditPopover from './KeyValueEditPopover.vue';
import { useKeyValueViewStore } from './state';
import { useKeyValueGate } from './useKeyValueGate';

const props = defineProps<{ viewKey: string }>();
const emit = defineEmits<{
  delete: [];
  download: [];
  upload: [];
  editObject: [];
}>();

const keyValueViewStore = useKeyValueViewStore();
const gate = useKeyValueGate(() => props.viewKey);
const { host, rt, page, connRecord } = gate;

const addPopover = useTemplateRef<InstanceType<typeof KeyValueAddPopover>>('addPopover');
const editPopover = useTemplateRef<InstanceType<typeof KeyValueEditPopover>>('editPopover');

const PAGE_SIZE_ITEMS = pageSizeOptions('keyvalue-').map((o) => ({
  value: String(o.value),
  label: o.label,
  testid: o.testid,
}));

// Redis pages are forward-only SCAN cursors: only a list's plain LRANGE offset can step back.
const prevDisabled = computed(
  () => (host.value?.pageIndex ?? 0) === 0 || page.value?.position.strategy !== 'offset',
);

const ttlLabel = computed(() => {
  const ttlMs = page.value?.ttlMs ?? null;
  return ttlMs === null
    ? t('studio.keyValue.noExpiry.status')
    : t('studio.keyValue.expires.status', {
        time: formatCompactDuration(ttlMs, { ceil: true }),
      });
});

const memoryLabel = computed(() => {
  const bytes = page.value?.memoryBytes ?? null;
  return bytes === null ? t('studio.keyValue.memoryUnknown.status') : formatBytes(bytes);
});

// Keys/fields, not table rows; SCAN-family cursors have no absolute range to show.
const statusLine = computed(() => {
  const r = rt.value;
  if (!r) return '';
  const parts: string[] = [t('studio.keyValue.status.loaded.status', { count: r.rowCount })];
  if (r.count) {
    parts.push(
      t(
        r.count.exact
          ? 'studio.keyValue.status.total.status'
          : 'studio.keyValue.status.totalApprox.status',
        { total: formatCount(r.count.value) },
      ),
    );
  }
  return parts.join(' · ');
});

defineExpose({
  openEdit: (value: string) => editPopover.value?.openWith(value),
  dirty: computed(() => !!addPopover.value?.dirty || !!editPopover.value?.dirty),
});
</script>

<template>
  <ViewToolbar v-if="page" data-testid="keyvalue-badges">
    <Badge data data-testid="keyvalue-type">{{ page.redisType }}</Badge>
    <Badge
      v-if="!gate.isSingleObjectPage.value"
      data
      :variant="page.ttlMs !== null ? 'warn' : 'chip'"
      data-testid="keyvalue-ttl"
    >
      <CodiconIcon :name="ICON.history" :size="13" />
      {{ ttlLabel }}
    </Badge>
    <Badge data data-testid="keyvalue-memory">{{ memoryLabel }}</Badge>
    <Badge v-if="connRecord">{{
      connRecord.readOnly ? t('common.state.readOnly.status') : t('studio.grid.writable.status')
    }}</Badge>
  </ViewToolbar>

  <ViewToolbar data-testid="keyvalue-toolbar">
    <div class="flex items-center gap-1.5 min-w-0">
      <TooltipIconButton
        v-if="!gate.isSingleObjectPage.value"
        icon="arrow-left"
        :label="t('common.action.previousPage.label')"
        disabled-trigger
        data-testid="keyvalue-prev"
        :disabled="prevDisabled"
        @click="keyValueViewStore.goPrev(viewKey)"
      />
      <span class="font-data text-kira-sm text-muted-foreground" data-testid="keyvalue-status">{{
        statusLine
      }}</span>
      <TooltipIconButton
        v-if="!gate.isSingleObjectPage.value"
        icon="arrow-right"
        :label="t('common.action.nextPage.label')"
        disabled-trigger
        data-testid="keyvalue-next"
        :disabled="!rt?.hasMore"
        @click="keyValueViewStore.goNext(viewKey)"
      />
    </div>

    <template v-if="!gate.isSingleObjectPage.value">
      <ToolbarSeparator />
      <SecondaryTabs
        variant="segmented"
        :model-value="String(host?.pageSize ?? 100)"
        :items="PAGE_SIZE_ITEMS"
        data-testid="keyvalue-page-size-picker"
        @update:model-value="(v) => keyValueViewStore.setPageSize(viewKey, Number(v) as PageSize)"
      />
    </template>

    <ToolbarSeparator />

    <div class="flex items-center gap-1.5 min-w-0">
      <TooltipIconButton
        icon="symbol-number"
        :label="t('studio.documents.count.run.aria')"
        data-testid="keyvalue-count"
        @click="keyValueViewStore.runCount(viewKey)"
      />
    </div>

    <ToolbarSeparator />

    <div class="flex items-center gap-1.5 min-w-0">
      <KeyValueAddPopover
        ref="addPopover"
        :view-key="viewKey"
        :disabled="!gate.canInsert.value"
        :label="gate.addTitle.value"
        :upload="gate.isSingleObjectPage.value"
        @upload="emit('upload')"
      />
      <KeyValueEditPopover
        ref="editPopover"
        :view-key="viewKey"
        :key-name="gate.keyName.value"
        :disabled="gate.editDisabled.value"
        :label="gate.editTitle.value"
        :object="gate.isSingleObjectPage.value"
        :current-value="gate.rowAt(0)?.value ?? ''"
        @edit-object="emit('editObject')"
      />
      <TooltipIconButton
        :icon="ICON.delete"
        :label="gate.deleteTitle.value"
        :aria-label="t('common.action.delete.label')"
        disabled-trigger
        :disabled="!gate.canDelete.value"
        data-testid="keyvalue-delete"
        @click="emit('delete')"
      />
      <TooltipIconButton
        v-if="gate.canDownload.value"
        icon="cloud-download"
        :label="gate.downloadTitle.value"
        :aria-label="t('common.action.download.label')"
        data-testid="keyvalue-download"
        @click="emit('download')"
      />
      <TooltipIconButton
        :icon="ICON.search"
        :label="t('studio.common.searchPage.label')"
        :pressed="!!rt?.searchOpen"
        data-testid="keyvalue-search"
        @click="keyValueViewStore.toggleSearchOpen(viewKey)"
      />
    </div>
  </ViewToolbar>
</template>
