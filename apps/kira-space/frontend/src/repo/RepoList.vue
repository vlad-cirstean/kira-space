<script setup lang="ts">
import type { WorktreeEntry } from '@kira/git-ipc';
import { PALETTE_COLOR_CHOICES } from '@shared/domain/color';
import type { RepoSummary } from '@shared/domain/repo';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { colorMarkClass, colourLabel } from '@theme/connColor';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { copyAndAnnounce } from '@workbench/behaviour/clipboard';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { matchesQuery } from '@workbench/behaviour/match';
import { useSortableReorder } from '@workbench/behaviour/sortableReorder';
import { useTreeShortcuts } from '@workbench/behaviour/treeShortcuts';
import { logError } from '@workbench/bridge/log';
import InlineEditInput from '@workbench/components/InlineEditInput.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import TextPromptDialog from '@workbench/prompt/TextPromptDialog.vue';
import { useTextPrompt } from '@workbench/prompt/useTextPrompt';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { useCodeReposStore } from '../state/coderepos';
import { openRepoTerminalTab } from '../state/repoTabs';
import { useTerminalsStore } from '../state/terminals';
import { repoWorkspaceKey, useWorkspaceStore } from '../state/workspace';
import RepoWorktreeRow from './RepoWorktreeRow.vue';
import { useRepoHeadsStore } from './state/repoHeads';
import { useRepoLinksStore } from './state/repoLinks';
import { useReposDialogStore } from './state/reposDialog';
import { useRepoVisibilityStore } from './state/repoVisibility';
import { useRepoWorktreeLists } from './state/worktreeQueries';
import { useWorktreesStore, worktreeLabel } from './state/worktrees';

type ColourName = Parameters<typeof colourLabel>[0];

// The Repos tab of the Git panel: every listed repository with its worktrees, rename, menus and
// keyboard navigation.
const props = defineProps<{ search: string }>();

const contextMenuStore = useContextMenuStore();
const codeReposStore = useCodeReposStore();
const workspaceStore = useWorkspaceStore();
const repoHeadsStore = useRepoHeadsStore();
const worktreesStore = useWorktreesStore();
const worktreeLists = useRepoWorktreeLists(() => worktreesStore.expandedRows);
const repoLinksStore = useRepoLinksStore();
const visibility = useRepoVisibilityStore();
const terminalsStore = useTerminalsStore();
const reposDialog = useReposDialogStore();

// Every top-level repo hidden and Show hidden off: the list is empty by choice, not for lack of imports.
const allHidden = computed(() => visibility.listed.length === 0 && visibility.hasHidden);

// A single click opens (if not yet open) or activates (if open). This panel's entire subject is
// repositories, so a click that only paints a highlight is a dead control; double-click still
// works, since it is a click first.
//
// An anchor row whose *worktree* is the open workspace must not read as closed — "open"
// now also means "some row parented to me is open". Guarded on `id` since worktreeRecordId can
// return '' for a worktree never opened in this app, and '' would otherwise match every open
// top-level repo (every anchor's own parentId is '' too).
function isOpen(id: string): boolean {
  if (!id) return false;
  return (
    workspaceStore.openRepos.includes(id) ||
    workspaceStore.openRepos.some((o) => repoLinksStore.worktreeParentId(o) === id)
  );
}
function isActive(id: string): boolean {
  return workspaceStore.active === repoWorkspaceKey(id);
}
function onRowClick(id: string): void {
  if (isOpen(id)) workspaceStore.activateWorkspace(repoWorkspaceKey(id));
  else workspaceStore.openRepoWorkspace(id);
}

/** P84 §6.1: the code_repos id backing a worktree path, once it has been opened in this app —
 *  '' before that (no row to mark open/active, no row to attach Rename/Close/Remove to yet). */
function worktreeRecordId(path: string): string {
  return codeReposStore.codeRepoRecordForPath(path)?.id ?? '';
}

// P84 §3: a row with a non-empty parentId never renders at the top level — only nested under its
// anchor's twisty (`worktreeEntries` below). §4.4: reads this panel's own PanelShell search box
// (`props.search`), not the Studio tree's own `treeState.search`.
const filteredRepos = computed<RepoSummary[]>(() => {
  const topLevel = visibility.listed;
  return topLevel.filter((r) => matchesQuery(r.name, props.search));
});

// P107 T2-19: shared in-app substitute for window.prompt() (Electron's renderer doesn't implement
// it) — see packages/workbench/src/prompt/useTextPrompt.ts.
const {
  prompt: textPrompt,
  open: promptText,
  submit: submitPrompt,
  cancel: cancelPrompt,
} = useTextPrompt();

// A repo row renames inline; a worktree's record name is not shown in its row, so that rename keeps
// the prompt.
const renamingRepoId = ref<string | null>(null);

function startRepoRename(id: string): void {
  contextMenuStore.afterClose(() => {
    renamingRepoId.value = id;
  });
}

async function onRenameWorktreeRecord(id: string, currentName: string): Promise<void> {
  const name = await promptText(t('space.repo.panel.rename.title'), currentName);
  if (!name || name.trim() === '') return;
  await codeReposStore.renameCodeRepo(id, name.trim());
}

async function commitRepoRename(id: string, name: string): Promise<void> {
  renamingRepoId.value = null;
  await codeReposStore.renameCodeRepo(id, name);
}

async function onRemoveRepo(id: string, name: string): Promise<void> {
  await codeReposStore.confirmRemoveCodeRepo(id, name);
}

// Same calls as the row's Close; collapse first so the lease release is synchronous.
function closeRepo(id: string): void {
  worktreesStore.collapseRepoWorktrees(id);
  workspaceStore.closeRepoWorkspace(id);
}

async function onHideRepo(id: string): Promise<void> {
  await codeReposStore.setCodeRepoHidden(id, true);
  closeRepo(id);
}

// The Rename…/Close/Remove triple onRepoContextMenu and onWorktreeContextMenu below
// both build — same three items, same handlers, only "which record" differs. Close only renders
// while `id` isOpen (both callers'); Rename/Remove always render (both callers already gate
// whether to offer this triple at all before calling — onWorktreeContextMenu only for a worktree
// that has its own code_repos record).
function recordMenuItems(
  record: { id: string; name: string },
  handlers: { rename: () => void; close: () => void; remove: () => void },
): MenuItem[] {
  const items: MenuItem[] = [
    {
      type: 'item' as const,
      id: 'rename',
      label: t('common.action.rename.label'),
      icon: 'edit',
      run: handlers.rename,
    },
  ];
  if (isOpen(record.id)) {
    items.push({
      type: 'item' as const,
      id: 'close',
      label: t('common.action.close.label'),
      icon: 'close',
      run: handlers.close,
    });
  }
  items.push({
    type: 'item' as const,
    id: 'remove',
    label: t('common.action.remove.label'),
    icon: 'trash',
    danger: true,
    run: handlers.remove,
  });
  return items;
}

function repoIconClass(repo: RepoSummary): string {
  return isOpen(repo.id) ? 'text-fg' : 'text-muted-foreground';
}

function repoMenuItems(repo: RepoSummary): MenuItem[] {
  return [
    {
      type: 'item' as const,
      id: 'open',
      label: t('common.action.open.label'),
      icon: 'folder-opened',
      run: () => onRowClick(repo.id),
    },
    {
      type: 'item' as const,
      id: 'copy-path',
      label: t('common.action.copyPath.label'),
      icon: 'copy',
      run: () => void copyAndAnnounce(repo.root, t('common.copy.copiedPath.announce')),
    },
    {
      type: 'item' as const,
      id: 'open-terminal',
      label: t('common.action.openTerminal.label'),
      icon: 'terminal-bash',
      run: () => void openRepoTerminalTab(repo.id, repo.root),
    },
    {
      type: 'item' as const,
      id: 'configure',
      label: t('space.repo.panel.configure.label'),
      icon: 'settings-gear',
      run: () => reposDialog.show({ repoId: repo.id }),
    },
    {
      type: 'submenu' as const,
      id: 'color',
      label: t('common.field.colour.label'),
      icon: 'symbol-color',
      items: PALETTE_COLOR_CHOICES.map((color) => ({
        type: 'item' as const,
        id: `color-${color}`,
        label: colourLabel(color as ColourName),
        swatch: color,
        checked: repo.color === color,
        run: () => codeReposStore.setCodeRepoColor(repo.id, color),
      })),
    },
    repo.hidden
      ? {
          type: 'item' as const,
          id: 'show',
          label: t('common.action.show.label'),
          icon: 'eye',
          run: () => codeReposStore.setCodeRepoHidden(repo.id, false),
        }
      : {
          type: 'item' as const,
          id: 'hide',
          label: t('common.action.hide.label'),
          icon: 'eye-closed',
          run: () => onHideRepo(repo.id),
        },
    { type: 'separator' as const },
    ...recordMenuItems(repo, {
      rename: () => startRepoRename(repo.id),
      // P82 §8.3: collapse first, so the lease release is synchronous with the close instead of a
      // watch flush later — the same reason quickOpen.ts:178 exposes dropQuickOpen alongside its
      // own watch. The §6.6 watch would collapse it anyway.
      close: () => closeRepo(repo.id),
      remove: () => onRemoveRepo(repo.id, repo.name),
    }),
  ];
}

// P83 §11.2: the indicator's own tooltip text — terminalCountAtPath is §11's whole signal (the
// terminal registry, already keyed by canonical cwd), so opening or exiting a terminal repaints
// the row with no event plumbing of this panel's own.
function terminalTooltip(n: number): string {
  return t('space.repo.panel.terminalOpen.tooltip', { count: n });
}

// A worktree row's own context menu — needed now since without it a terminal could never be opened
// at a worktree that isn't the active workspace. "Copy path" rides along since the menu must exist
// anyway and a one-item menu reads like an accident — the same item the repo row already offers
// above.
function worktreeMenuItems(repo: RepoSummary, wt: WorktreeEntry): MenuItem[] {
  const items: MenuItem[] = [
    {
      type: 'item' as const,
      id: 'open-terminal',
      label: t('common.action.openTerminal.label'),
      icon: 'terminal-bash',
      run: () => void openRepoTerminalTab(repo.id, wt.path),
    },
    {
      type: 'item' as const,
      id: 'copy-path',
      label: t('common.action.copyPath.label'),
      icon: 'copy',
      run: () => void copyAndAnnounce(wt.path, t('common.copy.copiedPath.announce')),
    },
  ];
  // P84 §6.2: once the flat top-level row is gone, Rename/Close/Remove for this record are
  // reachable from nowhere else — the same three items onRepoContextMenu builds, same handlers, no
  // second code path. Only offered once the worktree has its own code_repos row (it has been
  // opened in this app at least once); a worktree never opened here has nothing to rename or remove.
  const record = codeReposStore.codeRepoRecordForPath(wt.path);
  if (record) {
    items.push({ type: 'separator' as const });
    items.push(
      ...recordMenuItems(record, {
        rename: () => onRenameWorktreeRecord(record.id, record.name),
        close: () => closeRepo(record.id),
        // Removes the code_repos record, never the worktree on disk — the nested row survives it,
        // since it comes from `git worktree list`, not from codeReposState.
        remove: () => onRemoveRepo(record.id, record.name),
      }),
    );
  }
  return items;
}

// Flat row list for keyboard navigation: each repo followed by its worktrees while expanded.
type NavRow =
  | { key: string; kind: 'repo'; repo: RepoSummary }
  | { key: string; kind: 'worktree'; repo: RepoSummary; wt: WorktreeEntry };
const wtKey = (wt: WorktreeEntry): string => `wt:${wt.path}`;
const navRows = computed<NavRow[]>(() =>
  filteredRepos.value.flatMap((repo): NavRow[] => [
    { key: repo.id, kind: 'repo', repo },
    ...(worktreesStore.isWorktreesExpanded(repo.id)
      ? worktreeLists
          .entries(repo.id)
          .map((wt): NavRow => ({ key: wtKey(wt), kind: 'worktree', repo, wt }))
      : []),
  ]),
);
const listEl = useTemplateRef<HTMLElement>('listEl');
const activeNavKey = ref<string | null>(null);
// The roving stop follows the active workspace when it changes from elsewhere.
watch(
  () => workspaceStore.active,
  () => {
    const row = navRows.value.find((r) => r.kind === 'repo' && isActive(r.repo.id));
    if (row) activeNavKey.value = row.key;
  },
  { immediate: true },
);

function menuFor(row: NavRow): MenuItem[] {
  return row.kind === 'repo' ? repoMenuItems(row.repo) : worktreeMenuItems(row.repo, row.wt);
}
function activateRow(row: NavRow): void {
  if (row.kind === 'repo') onRowClick(row.repo.id);
  else void worktreesStore.switchToWorktree(row.repo.id, row.wt);
}
function onRowContextMenu(e: MouseEvent, row: NavRow): void {
  // Right-click focuses the row without opening anything.
  activeNavKey.value = row.key;
  contextMenuStore.openContextMenu(e, menuFor(row), {
    onRestoreFocus: () => void nav.focusKey(row.key),
  });
}

// Arrows, Home/End, Right/Left (worktrees), Enter and Space (open or switch), Shift+F10 come from
// the shared list navigation. No type-ahead: the panel's own type-to-search redirect owns printable keys.
const nav = useListNavigation<NavRow>({
  container: listEl,
  items: () => navRows.value,
  key: (row) => row.key,
  activeKey: activeNavKey,
  tree: {
    depth: (row) => (row.kind === 'repo' ? 0 : 1),
    isExpanded: (row) => row.kind === 'repo' && worktreesStore.isWorktreesExpanded(row.repo.id),
    hasChildren: (row) => row.kind === 'repo',
    expand: (row) => worktreesStore.toggleRepoWorktrees(row.repo.id),
    collapse: (row) => worktreesStore.toggleRepoWorktrees(row.repo.id),
  },
  onOpen: activateRow,
  onSelect: activateRow,
  onMenu: (row, rect) =>
    contextMenuStore.openContextMenuAt(rect.left, rect.bottom, menuFor(row), {
      onRestoreFocus: () => void nav.focusKey(row.key),
    }),
});

// Drag and Alt+arrows reorder top-level repos; a search shows a subset, so it pauses both.
useSortableReorder(
  listEl,
  () => filteredRepos.value.map((r) => r.id),
  (from, to) =>
    void codeReposStore
      .reorderCodeRepos(from, to)
      .catch((err) => logError('repoList', 'reorder failed', err)),
  {
    draggable: '.repo-entry',
    handle: '[data-testid="repo-row"]',
    keyboardHandle: '[data-testid="repo-row"]',
    keyboard: true,
    disabled: () => props.search.trim() !== '' || filteredRepos.value.length < 2,
  },
);

// F2 renames, Delete removes, Ctrl/Cmd+C copies the name of the focused row.
const rowRecord = (row: NavRow): { id: string; name: string } | undefined =>
  row.kind === 'repo' ? row.repo : codeReposStore.codeRepoRecordForPath(row.wt.path);
useTreeShortcuts(nav, {
  'tree.rename': (key) => {
    const row = navRows.value.find((r) => r.key === key);
    if (!row) return;
    if (row.kind === 'repo') startRepoRename(row.repo.id);
    else {
      const record = rowRecord(row);
      if (record) void onRenameWorktreeRecord(record.id, record.name);
    }
  },
  'tree.delete': (key) => {
    const row = navRows.value.find((r) => r.key === key);
    const record = row && rowRecord(row);
    if (record) void onRemoveRepo(record.id, record.name);
  },
  'tree.copyName': (key) => {
    const row = navRows.value.find((r) => r.key === key);
    if (row)
      void copyAndAnnounce(
        row.kind === 'repo' ? row.repo.name : worktreeLabel(row.wt),
        t('common.copy.copied.announce'),
      );
  },
});
</script>

<template>
  <EmptyState
    v-if="allHidden"
    fill
    icon="eye-closed"
    :title="t('space.repo.panel.allHidden.title')"
    :description="t('space.repo.panel.allHidden.description')"
    data-testid="repos-all-hidden"
  />
  <div
    v-else
    ref="listEl"
    class="flex flex-col"
    role="listbox"
    :aria-label="t('common.module.repositories.label')"
  >
    <div v-for="repo in filteredRepos" :key="repo.id" class="repo-entry">
      <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter, Space and the arrows come from useListNavigation on the list. -->
      <div
        :class="[
          cn(
            rowVariants({ layout: 'tree', selected: isActive(repo.id), muted: repo.hidden }),
            'h-row',
          ),
          { active: isActive(repo.id) },
        ]"
        :style="rowIndent(0)"
        data-testid="repo-row"
        :data-repo-id="repo.id"
        role="option"
        :data-nav-key="repo.id"
        :tabindex="nav.tabindexFor(repo.id)"
        :aria-selected="isActive(repo.id)"
        @focus="activeNavKey = repo.id"
        @click="onRowClick(repo.id)"
        @contextmenu.prevent="onRowContextMenu($event, { key: repo.id, kind: 'repo', repo })"
      >
        <span
          :class="colorMarkClass('rail', repo.color)"
          data-testid="repo-rail"
          aria-hidden="true"
        />
        <!-- P110 I2-13: RepoTreeRow.vue's own twisty, shared via TreeTwisty. -->
        <TreeTwisty
          :expanded="worktreesStore.isWorktreesExpanded(repo.id)"
          :has-children="true"
          :aria-expanded="worktreesStore.isWorktreesExpanded(repo.id)"
          testid="repo-row-expand"
          @toggle="worktreesStore.toggleRepoWorktrees(repo.id)"
        />
        <CodiconIcon
          name="source-control"
          :size="13"
          class="shrink-0"
          :class="repoIconClass(repo)"
        />
        <InlineEditInput
          v-if="renamingRepoId === repo.id"
          :name="repo.name"
          data-testid="repo-rename-input"
          @commit="(name) => commitRepoRename(repo.id, name)"
          @cancel="renamingRepoId = null"
        />
        <span v-else class="min-w-0 flex-1"
          ><TruncatedText :text="repo.root">{{ repo.name }}</TruncatedText></span
        >
        <!-- A repo row's checked-out branch. The name span's own flex-1
           above keeps it yielding first, so this is what survives on a narrow panel. Same
           size/colour
           pair as `.worktree-badge` below, so a collapsed row's branch and its expanded
           children's read as the same class of information. -->
        <Tooltip v-if="repo.hidden">
          <TooltipTrigger as-child>
            <CodiconIcon
              name="eye-closed"
              :size="13"
              class="shrink-0 text-subtle"
              data-testid="repo-hidden-mark"
            />
          </TooltipTrigger>
          <TooltipContent>{{ t('space.common.hidden.status') }}</TooltipContent>
        </Tooltip>
        <span
          v-if="repoHeadsStore.repoHeadLabel(repo.id)"
          class="repo-head min-w-0 max-w-5/12 flex-none text-kira-sm text-subtle"
          ><TruncatedText :text="repoHeadsStore.repoHeadLabel(repo.id)" /></span
        >
        <Tooltip v-if="terminalsStore.terminalCountAtPath(repo.root) > 0">
          <TooltipTrigger as-child>
            <CodiconIcon
              name="terminal-bash"
              :size="12"
              class="shrink-0 text-subtle"
              data-testid="repo-terminal-indicator"
            />
          </TooltipTrigger>
          <TooltipContent>{{
            terminalTooltip(terminalsStore.terminalCountAtPath(repo.root))
          }}</TooltipContent>
        </Tooltip>
      </div>
      <section
        v-if="worktreesStore.isWorktreesExpanded(repo.id)"
        class="flex flex-col"
        data-testid="repo-worktrees"
        :aria-label="t('common.module.worktrees.label')"
      >
        <RepoWorktreeRow
          v-for="wt in worktreeLists.entries(repo.id)"
          :key="wt.path"
          :wt="wt"
          :color="repo.color"
          :active="isActive(worktreeRecordId(wt.path))"
          :dimmed="!(wt.isCurrent || isOpen(worktreeRecordId(wt.path)))"
          :nav-key="wtKey(wt)"
          :tabindex="nav.tabindexFor(wtKey(wt))"
          @focus="activeNavKey = wtKey(wt)"
          @click="worktreesStore.switchToWorktree(repo.id, wt)"
          @menu="onRowContextMenu($event, { key: wtKey(wt), kind: 'worktree', repo, wt })"
        />
        <!-- Same indent as the worktree row above. -->
        <InlineState
          v-if="worktreesStore.switchError(repo.id) ?? worktreeLists.error(repo.id)"
          kind="error"
          :style="rowIndent(1)"
          data-testid="repo-worktree-error"
        >
          {{ worktreesStore.switchError(repo.id) ?? worktreeLists.error(repo.id) }}
        </InlineState>
        <InlineState
          v-else-if="worktreeLists.loading(repo.id) && worktreeLists.entries(repo.id).length === 0"
          kind="loading"
          :style="rowIndent(1)"
        >
          {{ t('common.state.loading.loading') }}
        </InlineState>
        <InlineState
          v-else-if="worktreeLists.entries(repo.id).length === 0"
          kind="empty"
          :style="rowIndent(1)"
        >
          {{ t('space.repo.panel.noWorktrees.empty') }}
        </InlineState>
      </section>
    </div>
  </div>

  <TextPromptDialog
    v-if="textPrompt"
    :title="textPrompt.title"
    :model-value="textPrompt.value"
    @update:model-value="(v) => textPrompt && (textPrompt.value = v)"
    @submit="submitPrompt"
    @close="cancelPrompt"
  />
</template>
