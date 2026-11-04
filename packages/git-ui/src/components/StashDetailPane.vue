<script setup lang="ts">
import type { CommitStore } from '@kira/git-core';
import { shortSha } from '@kira/git-core';
import EmptyState from '@theme/components/EmptyState.vue';
import Spinner from '@theme/components/Spinner.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldError } from '@theme/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { formatAbsoluteUtc, formatAgeCompact } from '@workbench/behaviour/format';
/**
 * "selecting a stash node or list row loads `stash.show` into the existing `FileTree.vue`".
 * Mirrors `DetailPane.vue`'s own composition but keyed off `StashState`
 * rather than `DetailState` — a stash entry has none of `CommitDetail`'s richer shape (body/
 * trailers/signature/multiple parents), so `CommitMeta.vue` is not reused here; this file's own
 * small header says the one thing worth saying about a stash (message, base commit, date, `-u`
 * marker) instead.
 *
 * No longer also composes `DiffView.vue` — opens the host's native diff editor now, exactly like
 * `DetailPane.vue`. The one real technical wrinkle `StashState` used to handle itself: a stash's
 * `-u` untracked files live only in its third parent (`entry.untrackedSha`), which has no
 * `baseSha` of its own — `editor.openDiff`'s own `fallbackSha` param carries that through to the
 * host handler now, which retries the whole composition against it when `path` is not among the
 * primary sha's changed files.
 */
import Identifier from '@workbench/components/Identifier.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed } from 'vue';
import { openFileAnnounced } from '../lib/openFileAnnounced.ts';
import type { DetailActions } from '../state/detailActions.ts';
import type { StashState } from '../state/stash.ts';
import FileTree from './FileTree.vue';

const props = defineProps<{
  stash: StashState;
  store: CommitStore;
  actions: DetailActions;
}>();

const entry = computed(() => props.stash.selected.value);
const files = computed(() => props.stash.changes.value ?? []);
/** Single-element — `FileTree.vue`'s own merge-parent picker only renders past `length > 1`, so
 *  this never shows one (a stash entry is never a merge from the UI's point of view: §7.6 truncates
 *  its parent list to `[baseSha]` in the graph, and the detail pane matches that). */
const parents = computed(() => (entry.value ? [entry.value.baseSha] : []));

/** G21 D12/D13: mirrors `DetailPane.vue`'s own `onOpenFile` — the one difference is `fallbackSha`,
 *  carrying `entry.untrackedSha` through so the host can retry an untracked file's composition
 *  against the stash's own third parent instead of throwing. */
function onOpenFile(index: number, pinned: boolean): void {
  const file = files.value[index];
  const current = entry.value;
  if (!file || !current) return;
  openFileAnnounced(props.actions, file.path, () =>
    props.actions.openInEditor({
      sha: current.sha,
      path: file.path,
      originalPath: file.originalPath,
      parentIndex: 0,
      pinned,
      ...(current.untrackedSha !== undefined ? { fallbackSha: current.untrackedSha } : {}),
    }),
  );
}
</script>

<template>
  <div class="flex flex-col min-h-0 h-full">
    <FieldError v-if="stash.error.value" class="p-3">
      {{ t('git.stash.loadError.error', { error: stash.error.value }) }}
    </FieldError>

    <template v-if="entry">
      <ViewToolbar data-testid="detail-head">
        <span class="min-w-0 text-kira-md text-fg"><TruncatedText :text="entry.message" /></span>
        <Badge><Identifier :value="entry.baseSha" :short="shortSha" /></Badge>
        <Badge v-if="entry.includedUntracked">{{ t('git.stash.untracked.example') }}</Badge>
        <Tooltip>
          <TooltipTrigger as-child>
            <span class="shrink-0 text-kira-sm text-subtle">{{
              formatAgeCompact(entry.timestamp)
            }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ formatAbsoluteUtc(entry.timestamp) }}</TooltipContent>
        </Tooltip>
      </ViewToolbar>
      <FileTree
        class="flex-auto min-h-0"
        :files="files"
        :selected-file="stash.selectedFile.value"
        :list-mode="stash.listMode.value"
        :filter="stash.filter.value"
        :parents="parents"
        :parent-index="0"
        :store="store"
        :actions="actions"
        @select-file="stash.selectFile($event)"
        @open-file="onOpenFile"
        @update:list-mode="stash.setListMode($event)"
        @update:filter="stash.setFilter($event)"
      />
    </template>

    <EmptyState v-else-if="!stash.error.value" :title="t('common.state.loading.loading')">
      <template #media><Spinner :size="24" label="" /></template>
    </EmptyState>
  </div>
</template>
