<script setup lang="ts">
import type { CommitStore, FileChangeKind } from '@kira/git-core';
import EmptyState from '@theme/components/EmptyState.vue';
/**
 * The uncommitted-changes strip's own click-through target — `DetailPane.vue`'s
 * sibling for the one case that pane can never represent (the working tree is not a commit: no
 * sha, no author, no parents). Deliberately NOT a mode of `DetailPane.vue` itself — that pane's
 * other half, `CommitMeta.vue`, has nothing to show here (every field it renders is commit-only),
 * so bolting an `undefined`-heavy commit shape onto it would cost more than this small, dedicated
 * pane does. `store`/`actions` are threaded straight through from whatever mounts this (`App.vue`),
 * the exact same instances `DetailPane.vue`/`StashDetailPane.vue` already use — `FileTree.vue`
 * only ever reads `actions.copy`/`actions.announce`/`actions.capabilities` from them (never
 * `openInEditor`/`openAllChanges`, both commit-shaped), so sharing the instance costs nothing and
 * keeps "copy path"/announcements consistent across every pane in the app.
 */
import { t } from '@theme/i18n';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed } from 'vue';
import { openFileAnnounced } from '../lib/openFileAnnounced.ts';
import type { DetailActions } from '../state/detailActions.ts';
import type { WorkingDetailState } from '../state/working.ts';
import FileTree from './FileTree.vue';

const props = defineProps<{
  workingState: WorkingDetailState;
  store: CommitStore;
  actions: DetailActions;
  /** Calls `editor.openWorkingDiff` — kept as a narrow callback prop, not a widening of
   *  `DetailActions` (whose `openInEditor` is shaped for a commit's sha/parentIndex, neither of
   *  which the working tree has). */
  openFile: (params: {
    path: string;
    originalPath: string | undefined;
    status: FileChangeKind;
    pinned: boolean;
  }) => Promise<void>;
}>();

const files = computed(() => props.workingState.files.value);

function onOpenFile(index: number, pinned: boolean): void {
  const file = files.value[index];
  if (!file) return;
  openFileAnnounced(props.actions, file.path, () =>
    props.openFile({
      path: file.path,
      originalPath: file.originalPath,
      status: file.kind,
      pinned,
    }),
  );
}
</script>

<template>
  <div class="flex flex-col min-h-0 h-full">
    <ViewToolbar data-testid="detail-head">
      <span class="min-w-0 truncate text-kira-md text-fg">{{ t('git.detail.working.title') }}</span>
    </ViewToolbar>
    <EmptyState
      v-if="workingState.error.value"
      fill
      icon="warning"
      :title="t('common.state.loadFailed.title')"
      :description="t('git.detail.working.error', { error: workingState.error.value })"
    />
    <FileTree
      v-else
      class="flex-auto min-h-0"
      :files="files"
      :selected-file="workingState.selectedFile.value"
      :list-mode="workingState.listMode.value"
      :filter="workingState.filter.value"
      :parents="[]"
      :parent-index="0"
      :store="store"
      :actions="actions"
      @select-file="workingState.selectFile($event)"
      @open-file="onOpenFile"
      @update:list-mode="workingState.setListMode($event)"
      @update:filter="workingState.setFilter($event)"
    />
  </div>
</template>
