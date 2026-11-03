<script setup lang="ts">
import type { ReviewComment } from '@kira/git-ipc';
import type { MountHandle, ReviewApi } from '@kira/git-ui';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { useTimeoutFn, useWindowFocus } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { logError } from '@workbench/bridge/log';
import { onMounted, onUnmounted, ref, shallowRef, watch } from 'vue';
import { loadGitUi } from '../../../repo/git/gitUiModule';
import { useGitRepoHold } from '../../../repo/git/repoHolds';
import { notifyReviewRepaint } from '../../../repo/git/reviewRepaint';
import { gitTransportFor } from '../../../repo/git/transport';
import { openRepoWorkingReviewTab } from '../../../state/repoTabs';
import { useReviewNavigationStore } from '../../../state/reviewNavigation';
import type { ReviewWindowTarget } from '../wire';
import {
  invalidateReviewWorking,
  useReviewWorking,
  useReviewWorkingMark,
  useReviewWorkingStamp,
} from './useReviewWorking';

// The review window's left pane: git-ui's review view locked to this window's branch and base.
const props = defineProps<{ target: ReviewWindowTarget }>();

const repoHold = useGitRepoHold(() => props.target.gitRepoId);
const errorMessage = ref('');
const container = ref<HTMLElement | null>(null);
let handle: MountHandle | null = null;
const api = shallowRef<ReviewApi | null>(null);
let unregisterNavigator: (() => void) | null = null;
const navigation = useReviewNavigationStore();

// Opens a file's diff in the centre tabs, as a click in the file list does.
defineExpose({
  selectFile: (path: string, origin: ReviewComment['origin'] = 'commit') =>
    origin === 'working'
      ? openRepoWorkingReviewTab(props.target.codeRepoId, path, props.target.branchId, false)
      : api.value?.selectFile(path),
});

// A missed watcher event (sleep, dropped FS event) heals when the window regains focus.
const focused = useWindowFocus();
watch(focused, (now, was) => {
  if (now && !was) api.value?.refresh();
});

// The uncommitted section under the committed list; git-ui renders it, this window feeds it.
const working = useReviewWorking(() => props.target.branchId);
const mark = useReviewWorkingMark();
// An edit no agent hook announced: poll a stat stamp of every listed path.
useReviewWorkingStamp(
  () => props.target.branchId,
  () => [
    ...(api.value?.committedPaths() ?? []),
    ...(working.data.value?.files.map((f) => f.path) ?? []),
  ],
  () => working.data.value?.worktree !== '' && api.value !== null,
);

// Every open diff of these paths re-reads its working review.
function repaintWorking(paths: readonly string[]): void {
  notifyReviewRepaint({
    kind: 'mark',
    scope: 'working',
    repoId: props.target.gitRepoId,
    branch: props.target.branch,
    paths,
  });
}

// The server refused a list mark because a file changed after the list was read.
const markRefused = ref(false);
const hideRefused = useTimeoutFn(() => (markRefused.value = false), TIMING.postedStatus, {
  immediate: false,
});

async function toggleWorking(paths: readonly string[], reviewed: boolean): Promise<void> {
  // Hashes of the list the user is looking at; a file with none (a list still loading) is unchecked.
  const shown = working.data.value?.contentHashes ?? {};
  const contentHashes = Object.fromEntries(
    paths.flatMap((p) => (shown[p] === undefined ? [] : [[p, shown[p]]])),
  );
  try {
    await mark.mutateAsync({
      branchId: props.target.branchId,
      paths: [...paths],
      reviewed,
      ranges: null,
      contentHashes,
    });
  } catch (err) {
    logError('adeReviewFiles', 'uncommitted review mark failed', err);
    if ((err as { code?: string }).code === 'E_CONFLICT') {
      markRefused.value = true;
      hideRefused.start();
      void invalidateReviewWorking();
    }
    return;
  }
  repaintWorking(paths);
}

function pushWorking(): void {
  const data = working.data.value;
  handle?.setWorkingChanges?.(
    data && data.worktree !== '' && data.files.length > 0
      ? {
          files: data.files,
          reviewStates: new Map(Object.entries(data.reviews)),
          onOpen: (path, pinned) =>
            openRepoWorkingReviewTab(props.target.codeRepoId, path, props.target.branchId, pinned),
          onToggleReviewed: (path, reviewed) => void toggleWorking([path], reviewed),
          onToggleReviewedFolder: (paths, reviewed) => void toggleWorking(paths, reviewed),
        }
      : null,
  );
}
watch(() => working.data.value, pushWorking);

// A commit moved reviewed work onto the committed review: refresh its list and its open diffs.
watch(
  () => working.data.value?.promoted,
  (promoted) => {
    if (!promoted || promoted.length === 0) return;
    api.value?.refresh();
    notifyReviewRepaint({
      kind: 'mark',
      scope: 'commit',
      repoId: props.target.gitRepoId,
      branch: props.target.branch,
      paths: promoted,
    });
    notifyReviewRepaint({
      kind: 'comments',
      repoId: props.target.gitRepoId,
      branch: props.target.branch,
    });
    repaintWorking(promoted);
  },
);

onMounted(async () => {
  if (!container.value) return;
  try {
    // The review view only asks about a repo this window already holds.
    await repoHold.ready();
    const transport = gitTransportFor(props.target.codeRepoId);
    const { mount, NullViewStateStore } = await loadGitUi();
    if (!container.value) return;
    handle = mount(container.value, {
      transport,
      viewState: new NullViewStateStore(),
      view: 'review',
      target: {
        repoId: props.target.gitRepoId,
        branch: props.target.branch,
        base: props.target.base,
        pane: 'files',
      },
      lockTarget: true,
      reviewShortcuts: true,
      onReviewApi: (a) => {
        api.value = a;
        unregisterNavigator?.();
        unregisterNavigator = navigation.register(props.target.codeRepoId, a);
      },
    });
    pushWorking();
  } catch (err) {
    errorMessage.value = errorText(err);
  }
});

onUnmounted(() => {
  unregisterNavigator?.();
  unregisterNavigator = null;
  handle?.unmount();
  handle = null;
});
</script>

<template>
  <div v-if="!errorMessage" class="flex h-full w-full flex-col">
    <Alert v-if="markRefused" variant="warn" data-testid="ade-review-files-mark-refused">
      <AlertDescription>{{ t('space.ade.review.working.markRefused.body') }}</AlertDescription>
    </Alert>
    <div ref="container" class="min-h-0 w-full flex-auto" data-testid="ade-review-files" />
  </div>
  <Alert v-else variant="info">
    <AlertDescription>{{ errorMessage }}</AlertDescription>
  </Alert>
</template>
