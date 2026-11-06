<script setup lang="ts">
import type { Caps } from '@shared/caps';
import type { PageSize } from '@shared/domain/tabs';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RunState from '@theme/RunState.vue';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref } from 'vue';
import type { DocumentTabRecord } from '../../../state/tabDomain';
import { useRunState } from '../../state/runState';
import PagerControls from '../shared/page/PagerControls.vue';
import { pageSizeOptions } from '../shared/page/sizes';
import ProjectionMenu from './ProjectionMenu.vue';
import { fieldNamesOnPage, pageVersion } from './page';
import { useDocumentViewStore } from './state';

const props = defineProps<{
  tab: DocumentTabRecord;
  caps: Caps | null;
  canInsert: boolean;
  insertTitle: string;
}>();
const emit = defineEmits<{
  refresh: [];
  expandAll: [];
  collapseAll: [];
  add: [];
}>();

const documentViewStore = useDocumentViewStore();
const rt = computed(() => documentViewStore.runtime[props.tab.id]);
const running = computed(() => rt.value?.status === 'loading');
const runState = useRunState(() => props.tab.id);

const PAGE_SIZE_ITEMS = pageSizeOptions('document-').map((o) => ({
  value: String(o.value),
  label: o.label,
  testid: o.testid,
}));

const projectionOpen = ref(false);
const projectionTriggerEl = ref<{ $el: HTMLElement } | null>(null);

const projectionCount = computed(() => {
  void pageVersion.n;
  const total = fieldNamesOnPage(props.tab.id).length;
  if (total === 0) return null;
  const projection = props.tab.state.projection;
  return { shown: projection ? projection.length : total, total };
});
</script>

<template>
  <ViewToolbar>
    <div class="flex items-center gap-1.5 min-w-0">
      <TooltipIconButton
        :icon="ICON.refresh"
        :label="t('common.action.refresh.label')"
        data-testid="document-refresh"
        @click="emit('refresh')"
      />
      <TooltipIconButton
        :icon="ICON.stop"
        :label="t('common.action.stop.label')"
        disabled-trigger
        :class="{ 'text-error': running }"
        data-testid="document-stop"
        :disabled="!running"
        @click="documentViewStore.stop(tab.id)"
      />
    </div>
    <ToolbarSeparator />
    <PagerControls
      :page-index="tab.state.pageIndex"
      :page-size="tab.state.pageSize"
      :count="rt?.count?.value ?? null"
      :has-more="!!rt?.hasMore"
      testid-prefix="document-"
      :last-tooltip="t('studio.documents.pager.countFirst.tooltip')"
      @first="documentViewStore.goFirst(tab.id)"
      @prev="documentViewStore.goPrev(tab.id)"
      @next="documentViewStore.goNext(tab.id)"
      @last="documentViewStore.goLast(tab.id)"
      @jump="(i) => documentViewStore.goToPage(tab.id, i)"
    />
    <SecondaryTabs
      variant="segmented"
      :model-value="String(tab.state.pageSize)"
      :items="PAGE_SIZE_ITEMS"
      data-testid="document-page-size-picker"
      @update:model-value="(v) => documentViewStore.setPageSize(tab.id, Number(v) as PageSize)"
    />
    <ToolbarSeparator />
    <!-- DataToolbar's [count, columns, preview] group — this collection's equivalents are
         the exact count, the fields/projection menu, and expand/collapse-all. -->
    <div class="flex items-center gap-1.5 min-w-0">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button
            variant="toolbar"
            size="kira-icon"
            :class="rt?.countError ? 'text-error' : ''"
            :aria-label="t('studio.documents.count.run.aria')"
            data-testid="document-count"
            @click="documentViewStore.runCount(tab.id)"
          >
            <CodiconIcon name="symbol-number" :size="13" />
          </Button>
        </TooltipTrigger>
        <TooltipContent data-testid="document-count-tooltip">{{
          rt?.countError
            ? t('studio.common.countFailed.tooltip', { error: rt.countError })
            : t('studio.documents.count.run.tooltip')
        }}</TooltipContent>
      </Tooltip>
      <div class="relative">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button
              ref="projectionTriggerEl"
              variant="toolbar"
              size="kira-icon"
              class="relative"
              :aria-expanded="projectionOpen"
              :aria-label="t('studio.common.fields.label')"
              data-testid="document-toolbar-projection"
              @click="projectionOpen = !projectionOpen"
            >
              <CodiconIcon name="list-selection" :size="13" />
              <StatusDot
                v-if="tab.state.projection !== null"
                tone="state-on"
                class="absolute top-0.5 right-0.5"
              />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{
            projectionCount
              ? t('studio.documents.projection.shown.tooltip', projectionCount)
              : t('studio.common.fields.label')
          }}</TooltipContent>
        </Tooltip>
        <Popover :open="projectionOpen" @update:open="(v) => (projectionOpen = v)">
          <PopoverAnchor
            :reference="(projectionTriggerEl?.$el as HTMLElement) ?? undefined"
            class="hidden"
          />
          <PopoverContent align="end" class="w-56 p-0" data-testid="document-projection-menu">
            <ProjectionMenu :tab-id="tab.id" :caps="caps" />
          </PopoverContent>
        </Popover>
      </div>
      <TooltipIconButton
        icon="expand-all"
        :label="t('common.action.expandAll.label')"
        data-testid="document-expand-all"
        @click="emit('expandAll')"
      />
      <TooltipIconButton
        icon="collapse-all"
        :label="t('common.action.collapseAll.label')"
        data-testid="document-collapse-all"
        @click="emit('collapseAll')"
      />
    </div>
    <ToolbarSeparator />
    <!-- DataToolbar's [add-row, delete-row, search] group — this collection has no delete
         affordance in the toolbar (deletion lives on the row's own context menu). -->
    <div class="flex items-center gap-1.5 min-w-0">
      <TooltipIconButton
        :icon="ICON.add"
        :label="insertTitle"
        :aria-label="t('studio.documents.add.label')"
        disabled-trigger
        :disabled="!canInsert"
        data-testid="document-add"
        @click="emit('add')"
      />
      <TooltipIconButton
        :icon="ICON.search"
        :label="t('studio.common.searchPage.label')"
        :pressed="!!rt?.searchOpen"
        data-testid="document-toolbar-search"
        @click="documentViewStore.toggleSearchOpen(tab.id)"
      />
    </div>
    <span class="ml-auto" />
    <Tooltip :disabled="true">
      <TooltipTrigger as-child>
        <RunState :state="runState" />
      </TooltipTrigger>
    </Tooltip>
  </ViewToolbar>
</template>
