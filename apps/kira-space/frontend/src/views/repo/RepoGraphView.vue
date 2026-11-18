<script setup lang="ts">
// The pinned first tab: the real @kira/git-ui graph mount, read-only — modelled on
// RepoDiffView.vue's own mount lifecycle: mount() on onMounted, unmount() on onUnmounted. MainView
// keeps this view mounted, hidden with v-show, from first activation until the tab closes
// (`keepMounted`); `active` drives git-ui's setVisible. Unmount means tab or workspace closed.
// Each mount holds its own lease over the per-workspace transport (gitTransportFor).
import type { MountHandle } from '@kira/git-ui';
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { onMounted, onUnmounted, ref, watch } from 'vue';
import { loadGitUi } from '../../repo/git/gitUiModule';
import { takePendingBlameReveal } from '../../repo/git/hostHandlers';
import { repoHeadOf } from '../../repo/git/repoHead';
import { gitTransportFor } from '../../repo/git/transport';
import { TabViewStateStore } from '../../repo/git/viewStateStore';
import { useCodeReposStore } from '../../state/coderepos';
import { useLayoutStore } from '../../state/layout';
import { useSettingsStore } from '../../state/settings';
import type { RepoGraphTabRecord } from '../../state/tabDomain';
import { noRepositoryMessage, repoIdOfTab } from '../../state/workspace';

const props = defineProps<{ tab: RepoGraphTabRecord; active?: boolean }>();

const errorMessage = ref('');
const container = ref<HTMLElement | null>(null);
let handle: MountHandle | null = null;
let stopSync: (() => void) | undefined;

const currentRepoHead = () =>
  repoHeadOf(useCodeReposStore().codeRepoRecord(repoIdOfTab(props.tab)));

async function mountGraph(): Promise<void> {
  const settingsStore = useSettingsStore();
  const layoutStore = useLayoutStore();
  const repoHead = currentRepoHead();
  const repoId = repoIdOfTab(props.tab);
  if (!repoId) {
    errorMessage.value = noRepositoryMessage();
    return;
  }
  if (!container.value) return; // unmounted (tab closed/switched away) before this ran.

  const { mount, parsePersistedViewState } = await loadGitUi();
  if (!container.value) return; // unmounted while the chunk above was in flight.

  // P62 §4.5: a blame-annotation click-through that fired while this tab was cold — consumed
  // exactly once, the same `pendingUiAction` seam G10 D19's palette commands already use.
  const pendingReveal = takePendingBlameReveal(repoId);

  handle = mount(container.value, {
    transport: gitTransportFor(repoId),
    viewState: new TabViewStateStore(props.tab.id, parsePersistedViewState),
    view: 'graph', // never 'review' — the C11 boundary (§9): this excludes the whole review layer.
    // P72 §9.1: app-wide appearance.dateFormat; later changes go through handle.update below.
    dateFormat: settingsStore.appearance.dateFormat,
    repoHead,
    pendingUiAction: pendingReveal ? { action: 'revealCommit', target: pendingReveal } : null,
    // P173: the failure banner's "Show in Operations" button.
    onShowOperations: () => {
      if (!layoutStore.panel.operations.visible) layoutStore.toggleOperationsPanel();
    },
  });
  // Covers a hide that landed while loadGitUi() was in flight.
  handle.setVisible?.(props.active !== false);
  stopSync = watch(
    () => [settingsStore.appearance.dateFormat, currentRepoHead()] as const,
    ([dateFormat, head]) => handle?.update?.({ dateFormat, repoHead: head }),
  );
}

// Hide reaches git-ui before display:none applies (CommitGrid's resize guard keys off it).
watch(
  () => props.active,
  (v) => {
    if (v === false) handle?.setVisible?.(false);
  },
  { flush: 'sync' },
);
// Show lands after the DOM is visible so the catch-up rebuild measures real size.
watch(
  () => props.active,
  (v) => {
    if (v !== false) handle?.setVisible?.(true);
  },
  { flush: 'post' },
);

onMounted(() => void mountGraph());

// Tab or workspace closed. The transport is cached per repo workspace (gitTransportFor); only
// closing the workspace disposes it. This mount's own transport is a lease — its dispose(), triggered by App.vue's own
// teardown chain on unmount, releases only this mount's subscriptions, never the shared socket.
onUnmounted(() => {
  stopSync?.();
  handle?.unmount();
  handle = null;
});
</script>

<template>
  <div v-if="!errorMessage" ref="container" class="h-full w-full" data-testid="repo-graph-host" />
  <EmptyState
    v-else
    fill
    icon="warning"
    :title="t('git.graph.loadFailed.error')"
    :description="errorMessage"
  />
</template>
