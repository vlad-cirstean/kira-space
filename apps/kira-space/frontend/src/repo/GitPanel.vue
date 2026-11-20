<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import { useKeptScroll, useKeptState } from '@workbench/behaviour/viewState';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { computed, onMounted, reactive, useTemplateRef, watch } from 'vue';
import { useCodeReposStore } from '../state/coderepos';
import { useLayoutStore } from '../state/layout';
import { repoIdOfWorkspace, useWorkspaceStore } from '../state/workspace';
import RepoList from './RepoList.vue';
import RepoReviewView from './RepoReviewView.vue';
import { useRepoPanelTabStore } from './state/panelTab';
import { useRepoHeadsStore } from './state/repoHeads';
import { useRepoLinksStore } from './state/repoLinks';
import { useRepoVisibilityStore } from './state/repoVisibility';

const codeReposStore = useCodeReposStore();
const workspaceStore = useWorkspaceStore();
const layoutStore = useLayoutStore();
const repoHeadsStore = useRepoHeadsStore();
const repoLinksStore = useRepoLinksStore();
const visibility = useRepoVisibilityStore();
const repoPanelTabStore = useRepoPanelTabStore();

// The Git module's own panel — one PanelShell, not a shell inside a shell. Absorbs the
// repository list that used to live in ProjectPanel.vue's "Connections" section (the
// complaint: "so there are no repos alingside connections") — it is now the title bar's former
// repo switcher, relocated and widened, sitting directly above whichever repo workspace's own
// Review body is active. `repoId` is still derived from the active workspace (never a
// prop) — '' when the bare 'git' key is active, which is exactly the "list only" state.
const repoId = computed(() => repoIdOfWorkspace(workspaceStore.active) ?? '');

const repoSearch = useKeptState('git:panel:repoSearch', '');
const reposEl = useTemplateRef<HTMLElement>('reposEl');
useKeptScroll('git:repos', reposEl);

// Tab persists (state/panelTab.ts, `kira.git.panelTab`, default Repos); no auto-switch on repo
// open or mount. With no repo workspace active, Review has nothing to show: display Repos
// without writing it, so the stored choice returns once a repo is active.
// Backed by useRepoPanelTabStore — hostHandlers.ts's review.open flips it from outside.
const tab = computed({
  get: () => (repoId.value ? repoPanelTabStore.repoPanelTab() : 'repos'),
  set: (v: 'repos' | 'review') => repoPanelTabStore.setRepoPanelTab(v),
});

const panelTabItems = computed(() => [
  { value: 'repos', label: t('common.module.repositories.label'), testid: 'git-panel-tab-repos' },
  {
    value: 'review',
    label: t('space.ade.board.branchTag.review.label'),
    testid: 'git-panel-tab-review',
  },
]);

// This panel is one persistent component instance across every repo workspace
// (WorkbenchShell.vue's `<component :is="activeModePanel" />` carries no per-repo key, unlike a
// tab), so "kept alive with v-show while the workspace is open" is tracked per repoId here
// rather than with a single boolean — switching to a DIFFERENT repo's workspace, or to another
// module and back, remounts (review.session.save/load plus the kept view state in
// useViewStateStore restore it) while switching between this SAME repo's Repos/Review tabs
// never does, since the set entry it added on first activation never goes away.
const reviewActivatedRepoIds = reactive(new Set<string>());
// A watcher, not the computed setter above, because setRepoPanelTab also has a second caller
// (hostHandlers.ts's review.open, via "Review branch changes") this component's own setter is
// never in the call path for.
watch(
  tab,
  (v) => {
    if (v !== 'review') return;
    reviewActivatedRepoIds.add(repoId.value);
    // §14 OQ2: widen once, only if the user has never manually resized the panel — checked inside
    // ensureReviewPanelWidth itself (state/layout.ts's own widthUserSet). 320px: VS Code's ~300px
    // sidebar default, raised a little for the review panes' own extra density.
    layoutStore.ensureReviewPanelWidth('git', 320);
  },
  { immediate: true },
);

// P104 §3: PanelShell's own search/empty props, inlined; only the Repos tab has a filter.
const panelSearch = computed<string>({
  get: () => repoSearch.value,
  set: (v: string) => {
    repoSearch.value = v;
  },
});
const panelEmpty = computed(() => codeReposStore.records.length === 0);
const panelSearchable = computed(() => tab.value !== 'review');
const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  stateKey: 'git:panel',
  searchable: () => panelSearchable.value,
  getSearch: () => panelSearch.value,
  setSearch: (v) => {
    panelSearch.value = v;
  },
});

onMounted(() => {
  // P83 plan §12.3 trigger 1: the panel is mounted for as long as the Git module is, so this is
  // once per session, not once per render.
  void repoHeadsStore.refreshRepoHeads();
  // P84 §4.4 trigger 1: same reasoning — one batched worktree-parent read per session, not once
  // per row.
  void repoLinksStore.refreshRepoWorktreeLinks();
});
</script>

<template>
  <div ref="rootEl" class="flex h-full min-h-0 flex-col">
    <PanelHeader>
      <!-- P84 §8.1/§9: replaces the old repo-name title — the tabs already say what's open. -->
      <template #start>
        <SecondaryTabs
          :model-value="tab"
          :items="panelTabItems"
          @update:model-value="(v) => (tab = v as 'repos' | 'review')"
        />
      </template>
      <template #actions>
        <TooltipIconButton
          icon="search"
          :label="
            showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
          "
          :pressed="showSearch"
          :kbd="formatShortcut('panel.find')"
          data-testid="toggle-search"
          @click="toggleSearch"
        />
        <TooltipIconButton
          v-if="tab === 'repos' && (visibility.hasHidden || visibility.showHidden)"
          :icon="visibility.showHidden ? 'eye' : 'eye-closed'"
          :label="
            visibility.showHidden
              ? t('space.repo.panel.hideHidden.tooltip')
              : t('space.repo.panel.showHidden.tooltip')
          "
          :pressed="visibility.showHidden"
          data-testid="repos-show-hidden"
          @click="visibility.showHidden = !visibility.showHidden"
        />
      </template>
    </PanelHeader>
    <template v-if="!panelEmpty">
      <PanelBar v-if="panelSearchable && showSearch">
        <SearchField v-model="panelSearch" data-testid="tree-search" />
      </PanelBar>
      <div class="min-h-0 flex-1">
        <div class="h-full flex flex-col min-h-0">
          <section
            v-if="tab === 'repos'"
            ref="reposEl"
            class="flex-1 min-h-0 overflow-y-auto"
            data-testid="repo-section"
          >
            <RepoList :search="repoSearch" />
          </section>
          <template v-else>
            <template v-if="repoId">
              <!-- C11 §8.4: mounted once (reviewActivatedRepoIds), then only ever hidden/shown,
                 never destroyed, by a Repos<->Review switch within this same repo. -->
              <RepoReviewView
                v-if="reviewActivatedRepoIds.has(repoId)"
                v-show="tab === 'review'"
                :key="repoId"
                class="flex-1 min-h-0"
                :repo-id="repoId"
              />
            </template>
            <EmptyState
              v-else
              fill
              icon="source-control"
              :title="t('space.repo.noRepoOpen.empty')"
            />
          </template>
        </div>
      </div>
    </template>
    <EmptyState
      v-else
      fill
      icon="source-control"
      :title="t('space.repo.panel.importPrompt.emptyHint')"
    />
  </div>

  <!-- Row state comes from `rowVariants` (selected/muted). `active` stays a real class on both
       rows (repo-workspace.spec.ts asserts `toHaveClass(/active/)`); `.repo-head`/`.worktree-badge`
       stay real classes too (the same spec locates them by class). -->
</template>
