<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { t } from '@theme/i18n';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import PanelBar from '@workbench/components/PanelBar.vue';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { registerCommand } from '@workbench/shortcuts/commands';
import { computed, onMounted, onUnmounted, reactive, useTemplateRef, watch } from 'vue';
import { useCodeReposStore } from '../state/coderepos';
import { useLayoutStore } from '../state/layout';
import { repoIdOfWorkspace, useWorkspaceStore } from '../state/workspace';
import RepoFileTree from './RepoFileTree.vue';
import RepoList from './RepoList.vue';
import RepoReviewView from './RepoReviewView.vue';
import RepoSearchView from './RepoSearchView.vue';
import { useFileTreeStore } from './state/fileTree';
import { useRepoHeadsStore } from './state/repoHeads';
import { useRepoLinksStore } from './state/repoLinks';
import { useRepoVisibilityStore } from './state/repoVisibility';
import { useRepoPanelTabStore, useRepoSearchStore } from './state/search';

const codeReposStore = useCodeReposStore();
const workspaceStore = useWorkspaceStore();
const layoutStore = useLayoutStore();
const repoHeadsStore = useRepoHeadsStore();
const fileTreeStore = useFileTreeStore();
const repoLinksStore = useRepoLinksStore();
const visibility = useRepoVisibilityStore();
const repoPanelTabStore = useRepoPanelTabStore();
const repoSearchStore = useRepoSearchStore();

// The Git module's own panel — one PanelShell, not a shell inside a shell. Absorbs the
// repository list that used to live in ProjectPanel.vue's "Connections" section (the
// complaint: "so there are no repos alingside connections") — it is now the title bar's former
// repo switcher, relocated and widened, sitting directly above whichever repo workspace's own
// Files/Search/Review body is active. `repoId` is still derived from the active workspace (never a
// prop) — '' when the bare 'git' key is active, which is exactly the "list only" state.
const repoId = computed(() => repoIdOfWorkspace(workspaceStore.active) ?? '');

// P84 §8.4: two independent queries — one string would mean a filter typed on one tab silently
// hides rows on the other. Each keeps its own text across tab switches.
const local = reactive({ repoSearch: '', fileSearch: '' });

// Tab persists (state/search.ts, `kira.git.panelTab`, default Repos); no auto-switch on repo
// open or mount. With no repo workspace active, Files/Review have nothing to show: display Repos
// without writing it, so the stored choice returns once a repo is active.
// Backed by useRepoPanelTabStore — hostHandlers.ts's review.open flips it from outside.
const tab = computed({
  get: () => (repoId.value ? repoPanelTabStore.repoPanelTab() : 'repos'),
  set: (v: 'repos' | 'files' | 'review') => repoPanelTabStore.setRepoPanelTab(v),
});

// C7 D9: lives in repo/state/search.ts, not component state, so switching workspaces and back
// does not reset it. P92 item 6: 'review' moved out to its own top-level `tab`, so this segment is
// back to Files/Search only.
const view = computed({
  get: () => repoSearchStore.repoSearchView(repoId.value),
  set: (v: 'files' | 'search') => repoSearchStore.setRepoSearchView(repoId.value, v),
});
const panelTabItems = computed(() => [
  { value: 'repos', label: t('common.module.repositories.label'), testid: 'git-panel-tab-repos' },
  { value: 'files', label: t('common.module.files.label'), testid: 'git-panel-tab-files' },
  {
    value: 'review',
    label: t('space.ade.board.branchTag.review.label'),
    testid: 'git-panel-tab-review',
  },
]);
const viewOptions = computed(() => [
  { value: 'files' as const, label: t('common.module.files.label'), testid: 'repo-view-files' },
  { value: 'search' as const, label: t('common.action.search.label'), testid: 'repo-view-search' },
]);

// This panel is one persistent component instance across every repo workspace
// (WorkbenchShell.vue's `<component :is="activeModePanel" />` carries no per-repo key, unlike a
// tab), so "kept alive with v-show while the workspace is open" is tracked per repoId here
// rather than with a single boolean — switching to a DIFFERENT repo's workspace and back remounts
// (review.session.save/load is exactly the resume path that makes that safe) while switching
// between this SAME repo's Files/Search/Review segments never does, since the set entry it added
// on first activation never goes away.
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

function onRefresh(): void {
  void fileTreeStore.refreshRepoTree(repoId.value);
}

// P104 §3: PanelShell's own search/empty props, inlined -- the panel routes to one of two search
// strings depending on which top-level tab is active (Repos vs. Files), same as before.
const panelSearch = computed<string>({
  get: () => (tab.value === 'repos' ? local.repoSearch : local.fileSearch),
  set: (v: string) => {
    if (tab.value === 'repos') local.repoSearch = v;
    else local.fileSearch = v;
  },
});
const panelEmpty = computed(() => codeReposStore.records.length === 0);
const panelSearchable = computed(() => tab.value !== 'review');
const rootEl = useTemplateRef<HTMLElement>('rootEl');
const { showSearch, toggleSearch } = usePanelHeaderSearch(rootEl, {
  searchable: () => panelSearchable.value,
  getSearch: () => panelSearch.value,
  setSearch: (v) => {
    panelSearch.value = v;
  },
});

// C7 S10: `repo.search`'s own palette entry (shortcuts/state.ts) — this panel is the whole of a
// repo workspace's own left panel, mounted for as long as that workspace is open, so there is no
// tab-scoping question the way view.find's per-view registration has.
let unregisterSearchCommand: (() => void) | null = null;
onMounted(() => {
  unregisterSearchCommand = registerCommand('repo.search', () => {
    view.value = 'search';
  });
  // P83 plan §12.3 trigger 1: the panel is mounted for as long as the Git module is, so this is
  // once per session, not once per render.
  void repoHeadsStore.refreshRepoHeads();
  // P84 §4.4 trigger 1: same reasoning — one batched worktree-parent read per session, not once
  // per row.
  void repoLinksStore.refreshRepoWorktreeLinks();
});
onUnmounted(() => {
  unregisterSearchCommand?.();
  unregisterSearchCommand = null;
});
</script>

<template>
  <div ref="rootEl" class="flex h-full min-h-0 flex-col">
    <PanelHeader>
      <!-- P84 §8.1/§9: replaces the old repo-name title — the tabs already say what's open.
           P92 item 6: Review joins Repos/Files as a third tab, off the Files body's own segment. -->
      <template #start>
        <SecondaryTabs
          :model-value="tab"
          :items="panelTabItems"
          @update:model-value="(v) => (tab = v as 'repos' | 'files' | 'review')"
        />
      </template>
      <template #actions>
        <TooltipIconButton
          icon="search"
          :label="
            showSearch ? t('common.action.hideSearch.label') : t('common.action.search.label')
          "
          :pressed="showSearch"
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
        <!-- Files mode only: in Search mode the panel's own tree filter is meaningless, and a tree
             refresh has nothing to do with a search result list. -->
        <TooltipIconButton
          v-if="tab === 'files' && view === 'files'"
          icon="refresh"
          :label="t('space.repo.panel.refreshTree.tooltip')"
          :aria-label="t('common.action.refresh.label')"
          data-testid="repo-refresh"
          @click="onRefresh"
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
            class="flex-1 min-h-0 overflow-y-auto"
            data-testid="repo-section"
          >
            <RepoList :search="local.repoSearch" />
          </section>
          <template v-else>
            <template v-if="repoId">
              <template v-if="tab === 'files'">
                <PanelBar>
                  <SecondaryTabs
                    :model-value="view"
                    :items="viewOptions"
                    @update:model-value="(v) => (view = v as 'files' | 'search')"
                  />
                </PanelBar>
                <template v-if="view === 'files'">
                  <Alert
                    v-if="fileTreeStore.repoTreeError(repoId)"
                    variant="error"
                    data-testid="repo-tree-error"
                  >
                    <AlertDescription>{{ fileTreeStore.repoTreeError(repoId) }}</AlertDescription>
                  </Alert>
                  <Alert
                    v-if="fileTreeStore.repoTreeTruncated(repoId)"
                    variant="info"
                    data-testid="repo-tree-truncated"
                  >
                    <AlertDescription>{{
                      t('space.repo.panel.treeTruncated.hint')
                    }}</AlertDescription>
                  </Alert>
                  <!-- Always mounted, never gated on isRepoTreeLoaded — RepoFileTree's own
                     onMounted is what calls ensureRepoTreeLoaded in the first place; gating on the
                     state it sets would mean it never gets the chance to. Its own `rows` computed
                     is empty until the load resolves, then updates reactively — no separate loading
                     placeholder needed for a first open this fast. -->
                  <RepoFileTree
                    class="flex-1 min-h-0"
                    :repo-id="repoId"
                    :search="local.fileSearch"
                  />
                </template>
                <RepoSearchView
                  v-else-if="view === 'search'"
                  class="flex-1 min-h-0"
                  :repo-id="repoId"
                />
              </template>
              <!-- C11 §8.4: mounted once (reviewActivatedRepoIds), then only ever hidden/shown,
                 never destroyed, by a Files<->Review or Search<->Review switch within this same
                 repo. P92 item 6: gated on `tab`, Review's own top-level tab, not `view`. -->
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
