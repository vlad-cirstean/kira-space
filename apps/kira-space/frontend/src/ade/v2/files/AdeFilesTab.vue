<script setup lang="ts">
import { useQueryClient } from '@tanstack/vue-query';
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { NativeSelect } from '@theme/components/ui/native-select';
import { ResizeHandle } from '@theme/components/ui/resizable';
import { useLiveSize } from '@theme/components/ui/resizable/useLiveSize';
import { t } from '@theme/i18n';
import { useElementSize } from '@vueuse/core';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import PanelBar from '@workbench/components/PanelBar.vue';
import { computed, onUnmounted, useTemplateRef, watch } from 'vue';
import { control } from '../../../bridge/control';
import { requestReveal } from '../../../views/repo/reveal';
import { launchTargets } from '../board/taskLaunch';
import type { CardModel } from '../plan/usePlanModel';
import AdeFileSearch from './AdeFileSearch.vue';
import AdeFileTree from './AdeFileTree.vue';
import AdeFileViewer from './AdeFileViewer.vue';
import { useAdeFileSearchStore } from './state/fileSearch';
import { type FilesMode, useAdeFilesUiStore } from './state/filesUi';
import { taskFileTabId, useTaskFilesListing } from './useTaskFiles';

// A task's worktree files: tree or content search on the left, a read-only viewer on the right.
// One branch at a time; the worktree is the agent's, so nothing here edits.
const props = defineProps<{ card: CardModel }>();
const filesUi = useAdeFilesUiStore();
const searchStore = useAdeFileSearchStore();
const queryClient = useQueryClient();

const targets = computed(() =>
  launchTargets(props.card.rows, []).filter((target) => target.worktree !== ''),
);
const view = computed(() => filesUi.viewOf(props.card.task.id));
const branchId = computed(
  () =>
    targets.value.find((x) => x.branchId === view.value.branchId)?.branchId ??
    targets.value[0]?.branchId ??
    '',
);
const target = computed(() => targets.value.find((x) => x.branchId === branchId.value));
const branchOptions = computed(() =>
  targets.value.map((x) => ({ value: x.branchId, label: `${x.repo} · ${x.branch}` })),
);

const mode = computed({
  get: () => view.value.mode,
  set: (v: FilesMode) => filesUi.patch(props.card.task.id, { mode: v }),
});
const filter = computed({
  get: () => view.value.filter,
  set: (v: string) => filesUi.patch(props.card.task.id, { filter: v }),
});
const modeItems = computed(() => [
  { value: 'files', label: t('common.module.files.label'), testid: 'ade-files-mode-files' },
  { value: 'search', label: t('common.action.search.label'), testid: 'ade-files-mode-search' },
]);
const openFile = computed(() => filesUi.openFile[branchId.value] ?? null);

const listing = useTaskFilesListing(branchId);
const listingError = computed(() => (listing.error.value ? errorText(listing.error.value) : ''));

function refresh(): void {
  void queryClient.invalidateQueries({ queryKey: ['adetask', 'files', branchId.value] });
}

function onOpen(path: string): void {
  filesUi.openFile[branchId.value] = path;
}
function onSearchOpen(
  path: string,
  reveal: { line: number; column: number; endColumn?: number },
): void {
  filesUi.openFile[branchId.value] = path;
  requestReveal(taskFileTabId(branchId.value, path), reveal);
}

// The worktree session on the engine side lives while a tab shows it; leaving the branch or the
// tab releases it, with the branch's search.
function release(id: string): void {
  if (id === '') return;
  searchStore.cancelSearch(id);
  searchStore.dropSearch(id);
  fireAndForget(control.adeTaskFilesClose(id), 'files');
}
watch(branchId, (_next, prev) => {
  if (prev !== undefined) release(prev);
});
onUnmounted(() => release(branchId.value));

const DEFAULT_PANE = 220;
const MIN_PANE = 140;
const MIN_VIEWER = 160;
const body = useTemplateRef<HTMLElement>('body');
const { width: bodyWidth } = useElementSize(body);
const maxPane = computed(() => Math.max(MIN_PANE, bodyWidth.value - MIN_VIEWER));
const pane = useLiveSize(
  () => Math.max(MIN_PANE, Math.min(maxPane.value, filesUi.paneWidth || DEFAULT_PANE)),
  (w) => {
    filesUi.paneWidth = Math.round(w);
  },
);
</script>

<template>
  <EmptyState
    v-if="branchId === ''"
    fill
    icon="folder"
    :title="t('space.ade.files.noWorktree.empty')"
    data-testid="ade-files-empty"
  />
  <div v-else class="flex h-full min-h-0 flex-col" data-testid="ade-files-tab">
    <PanelBar class="flex-row flex-wrap items-center">
      <NativeSelect
        v-if="targets.length > 1"
        :aria-label="t('common.field.branch.label')"
        variant="bordered"
        :model-value="branchId"
        data-testid="ade-files-branch"
        @update:model-value="(v) => filesUi.patch(card.task.id, { branchId: String(v) })"
      >
        <option v-for="o in branchOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
      </NativeSelect>
      <SecondaryTabs
        variant="segmented"
        :model-value="mode"
        :items="modeItems"
        data-testid="ade-files-mode"
        @update:model-value="(v) => (mode = v as FilesMode)"
      />
      <div v-if="mode === 'files'" class="min-w-24 flex-1">
        <SearchField
          v-model="filter"
          :placeholder="t('git.fileTree.filter.placeholder')"
          data-testid="ade-files-filter"
        />
      </div>
      <div v-else class="flex-1" />
      <TooltipIconButton
        icon="refresh"
        :label="t('space.ade.files.refresh.tooltip')"
        data-testid="ade-files-refresh"
        @click="refresh"
      />
    </PanelBar>
    <Alert v-if="listingError" variant="error" data-testid="ade-files-error"
      ><AlertDescription>{{ listingError }}</AlertDescription></Alert
    >
    <Alert
      v-else-if="listing.data.value?.truncated"
      variant="info"
      data-testid="ade-files-truncated"
      ><AlertDescription>{{ t('space.ade.files.truncated.hint') }}</AlertDescription></Alert
    >
    <div ref="body" class="flex min-h-0 flex-1">
      <div class="min-h-0 shrink-0" :style="{ width: `${pane.size}px` }">
        <AdeFileTree
          v-if="mode === 'files'"
          :branch-id="branchId"
          :worktree="target?.worktree ?? ''"
          :filter="filter"
          @open="onOpen"
        />
        <AdeFileSearch v-else :branch-id="branchId" @open="onSearchOpen" />
      </div>
      <ResizeHandle
        line
        :label="t('space.ade.files.resize.label')"
        data-testid="ade-files-resize-handle"
        :min="MIN_PANE"
        :max="maxPane"
        v-bind="pane.bind"
      />
      <div class="min-h-0 min-w-0 flex-1">
        <AdeFileViewer
          v-if="openFile"
          :key="`${branchId}:${openFile}`"
          :branch-id="branchId"
          :path="openFile"
        />
        <EmptyState
          v-else
          fill
          icon="file"
          :title="t('space.ade.files.viewer.empty')"
          data-testid="ade-files-viewer-empty"
        />
      </div>
    </div>
  </div>
</template>
