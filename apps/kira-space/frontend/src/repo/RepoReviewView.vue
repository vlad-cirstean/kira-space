<script setup lang="ts">
// The review sidebar's real mount, modelled on views/repo/RepoGraphView.vue's own mount lifecycle
// (view: 'graph' there, 'review' here) with two differences: this is not a tab (GitPanel.vue keeps
// it mounted with v-show while the segment is hidden rather than unmounting it on every switch
// away, so an in-flight review survives a trip to Files and back) and `viewState` is
// `NullViewStateStore` — `ReviewView.vue` never reads it, so there is no per-mount state to persist
// through a TabViewStateStore the way the graph needs one.
import type { MountHandle } from '@kira/git-ui';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { onMounted, onUnmounted, ref } from 'vue';
import { useCodeReposStore } from '../state/coderepos';
import { useReviewNavigationStore } from '../state/reviewNavigation';
import { loadGitUi } from './git/gitUiModule';
import { takePendingReviewTarget } from './git/hostHandlers';
import { repoHeadOf } from './git/repoHead';
import { gitTransportFor } from './git/transport';

const props = defineProps<{ repoId: string }>();

const errorMessage = ref('');
const container = ref<HTMLElement | null>(null);
let handle: MountHandle | null = null;
let unregisterNavigator: (() => void) | null = null;
const navigation = useReviewNavigationStore();

async function mountReview(): Promise<void> {
  if (!props.repoId) {
    errorMessage.value = t('space.repo.review.noRepo.error');
    return;
  }
  if (!container.value) return; // unmounted (workspace closed) before this ran.

  // §8.4/S13: review.open's own cold-mount hand-off (hostHandlers.ts) — consumed once, so a later
  // remount (there isn't one, this component mounts exactly once per workspace, S14) never
  // re-applies a stale target.
  const target = takePendingReviewTarget(props.repoId);

  const { mount, NullViewStateStore } = await loadGitUi();
  if (!container.value) return; // unmounted while the chunk above was in flight.

  handle = mount(container.value, {
    transport: gitTransportFor(props.repoId),
    viewState: new NullViewStateStore(),
    view: 'review',
    repoHead: repoHeadOf(useCodeReposStore().codeRepoRecord(props.repoId)),
    target,
    onReviewApi: (api) => {
      unregisterNavigator?.();
      unregisterNavigator = navigation.register(props.repoId, api);
    },
  });
}

onMounted(() => void mountReview());

// Only fires on workspace close (GitPanel.vue keeps this component alive with v-show for a mere
// segment switch) — the transport itself is cached per repo workspace and outlives this
// regardless (gitTransportFor, disposed by state/workspace.ts's own close path). This
// view's own transport is a lease: its dispose() on unmount releases only this
// mount's subscriptions, so it can no longer take the graph tab's transport down with it.
onUnmounted(() => {
  unregisterNavigator?.();
  unregisterNavigator = null;
  handle?.unmount();
  handle = null;
});
</script>

<template>
  <div v-if="!errorMessage" ref="container" class="h-full w-full" data-testid="repo-review-host" />
  <EmptyState
    v-else
    fill
    icon="warning"
    :title="t('common.state.loadFailed.title')"
    :description="errorMessage"
  />
</template>
