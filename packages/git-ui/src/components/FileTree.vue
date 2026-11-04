<script setup lang="ts">
import type { CommitStore } from '@kira/git-core';
/**
 * The file tree, its tree/flat toggle, its filter box, and (in this component's own header) the
 * merge parent selector. Rendering itself is the only thing this file adds on top of
 * `fileTreeModel.ts`'s pure fold; every piece of "what row goes where" is that module's job, not
 * this one's.
 *
 * One component, one anatomy. The review-only-styling prop added so this
 * file could render *two different anatomies* (a dimmed flat-mode directory suffix, an inline
 * copy button vs. a right-click "Copy path" menu) depending on caller, in service of a
 * byte-identity guarantee the graph panel's tree needed while it still embedded a diff — is gone.
 * The graph tree's icons, status glyph, and click/double-click behaviour already changed; there is
 * no byte-identity guarantee left to protect, so the review anatomy (the row geometry
 * `ReviewView.vue` used to restyle from outside under `skin-kira`, and the
 * dimmed-directory-suffix/context-menu-copy behaviour) is now this component's *only* appearance,
 * everywhere it mounts — `DetailPane.vue`, `StashDetailPane.vue`, `ReviewFilesPane.vue`,
 * `ReviewCommitRow.vue` alike.
 */
import type { FileChange, ReviewFileStatus } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { t } from '@theme/i18n';
import { rowIndent, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useCappedList } from '@workbench/behaviour/cappedList';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import { usePanelHeaderSearch } from '@workbench/behaviour/panelSearch';
import { errorText } from '@workbench/bridge/codedError';
import ShowMoreButton from '@workbench/components/ShowMoreButton.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, ref, watch } from 'vue';
import { setiIconFor } from '../icons/setiFileIcon.ts';
import {
  buildFileTree,
  buildFlatList,
  FILE_TREE_PAGE_SIZE,
  type FileTreeRow,
  filterFiles,
  flattenTreeRows,
  renameDisplay,
  STATUS_COLOR_CLASS,
  STATUS_LETTERS,
} from '../lib/fileTreeModel.ts';
import { buildFolderReview, folderToggle } from '../lib/folderReview.ts';
import { sectionsToMenuItems } from '../lib/menuModel.ts';
import {
  checkboxState,
  nameTone,
  type ReviewAttention,
  reviewAttention,
  toggleTitleKey,
} from '../lib/reviewAttention.ts';
import { buildFileRowMenu } from '../lib/rowMenuModel.ts';
import type { FileListMode } from '../state/detail.ts';
import type { DetailActions } from '../state/detailActions.ts';
import FileTreeToolbar from './FileTreeToolbar.vue';

const props = defineProps<{
  files: readonly FileChange[];
  selectedFile: number;
  listMode: FileListMode;
  filter: string;
  parents: readonly string[];
  parentIndex: number;
  store: CommitStore;
  actions: DetailActions;
  /** The commit these files belong to — enables the row menu's "Go to file"
   *  (`actions.goToFile({ rev: sha, path, line: 1 })`). Absent
   *  renders byte-identically: the item does not appear, the same "absent, not disabled"
   *  convention `reviewStates` above already uses. */
  sha?: string;
  /** The review sidebar's Files pane only — a per-path reviewed status, keyed by
   *  `FileChange.path`. Absent (the default, every other caller of this component) renders
   *  byte-identically to before this prop existed: no checkbox, no badge, nothing —
   *  `DetailPane.vue` and `StashDetailPane.vue` are provably unaffected. */
  reviewStates?: ReadonlyMap<string, ReviewFileStatus>;
  /** Whether this instance renders its own filter/list-mode toolbar. `false` when a
   *  panel above it owns one for several trees at once (the review sidebar) — `DetailPane.vue`,
   *  which mounts exactly one tree, leaves this unset and gets today's behaviour byte for byte.
   *  A real layout fact, not an anatomy fork: the review panel owns one
   *  toolbar for two panes, the graph and stash panels each own exactly one tree, so the toolbar
   *  belongs to whichever owns it — unlike the review-styling prop this file used to also carry. */
  showToolbar?: boolean; // default true
}>();

const emit = defineEmits<{
  /** The file cursor moved (a click, arrow-key nav, or a directory toggle's own side effect) —
   *  purely a cursor/selection signal now: no caller may infer that anything opened
   *  from this alone. `DetailPane.vue`/`StashDetailPane.vue` forward it straight into
   *  `DetailState`/`StashState`'s own `selectFile`, which only moves the highlight. */
  (e: 'selectFile', fileIndex: number): void;
  /** A file row was opened — a click or arrow-key move (`pinned: false`, navigational:
   *  the preview-tab convention governs, the next such open replaces it) or a double
   *  click/`Enter` (`pinned: true`, an explicit "keep this" that pins a real, permanent tab).
   *  Always paired with a `selectFile` for the same `fileIndex` when it originates from
   *  `selectRow` (a click or arrow-key move); a double click/`Enter` fires this alone, since the
   *  row is already the selection by the time either can happen. */
  (e: 'openFile', fileIndex: number, pinned: boolean): void;
  (e: 'update:listMode', mode: FileListMode): void;
  (e: 'update:filter', text: string): void;
  (e: 'update:parentIndex', index: number): void;
  /** The reviewed checkbox's own click — toggles path between fully reviewed and
   *  unreviewed (the file-level toggle; per-range toggling lives in the native diff editor now,
   *  it used to live in DiffView.vue's own review adornment). Never emitted when reviewStates is
   *  absent. */
  (e: 'toggleReviewed', path: string): void;
  /** A directory checkbox's click: the files under it that change, and the state to set. */
  (e: 'toggleReviewedFolder', paths: string[], reviewed: boolean): void;
}>();

const filterInput = ref(props.filter);
watch(
  () => props.filter,
  (value) => {
    filterInput.value = value;
  },
);
function onFilterInput(value: string): void {
  filterInput.value = value;
  emit('update:filter', value);
}

const collapsedDirs = ref<Set<string>>(new Set());
function isExpanded(path: string): boolean {
  return !collapsedDirs.value.has(path);
}
function toggleDir(path: string): void {
  const next = new Set(collapsedDirs.value);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  collapsedDirs.value = next;
}
function expandDir(path: string): void {
  if (!collapsedDirs.value.has(path)) return;
  const next = new Set(collapsedDirs.value);
  next.delete(path);
  collapsedDirs.value = next;
}
function collapseDir(path: string): void {
  if (collapsedDirs.value.has(path)) return;
  const next = new Set(collapsedDirs.value);
  next.add(path);
  collapsedDirs.value = next;
}

const indexed = computed(() => filterFiles(props.files, props.filter));

// `tree` depends only on `indexed` (the files/filter),
// never on `collapsedDirs` — split out from `rows` so toggling a single twisty re-runs only the
// cheap flatten below, not buildFileTree's own full O(n log n) fold (a fresh DirBuilder Map per
// directory, a sort at every level, a fresh node object per file) over every file in the commit.
const tree = computed(() => buildFileTree(indexed.value));

const rows = computed<FileTreeRow[]>(() => {
  if (props.listMode === 'flat') {
    return buildFlatList(indexed.value).map((node) => ({
      kind: 'file' as const,
      node,
      depth: 0,
    }));
  }
  return flattenTreeRows(tree.value, isExpanded);
});

/** §8's render cap; a fresh commit selection gets the first page again. */
const {
  visible: visibleRows,
  hiddenCount,
  showMore,
  reset: resetCap,
  step: capStep,
} = useCappedList(rows, FILE_TREE_PAGE_SIZE);
watch(() => props.files, resetCap);

const treeEl = ref<HTMLDivElement | null>(null);
const rootEl = ref<HTMLElement | null>(null);
const activeKey = ref<string | null>(null);

// Type to search: a printable key on the tree redirects into the filter field.
usePanelHeaderSearch(rootEl, {
  searchable: () => props.showToolbar !== false,
  getSearch: () => filterInput.value,
  setSearch: onFilterInput,
  searchTestid: 'file-filter',
});

/** Keeps the keyboard cursor in step with `selectedFile` however it changed — a mouse click on a
 *  row (handled locally, below), or `selectedFile` being driven from outside this component
 *  entirely (a parent-commit pick, a repo refresh re-resolving a selection). A `selectedFile` the
 *  current filter/list-mode has hidden leaves the cursor where it was. */
watch(
  [() => props.selectedFile, visibleRows],
  ([selectedFile]) => {
    const rowsNow = visibleRows.value;
    const selected = rowsNow.find(
      (row) => row.kind === 'file' && row.node.fileIndex === selectedFile,
    );
    if (selected) activeKey.value = rowKey(selected);
    else if (!rowsNow.some((row) => rowKey(row) === activeKey.value)) {
      activeKey.value = rowsNow[0] ? rowKey(rowsNow[0]) : null;
    }
  },
  { immediate: true },
);

/** P5 W14: announces §8's render cap once per boundary crossing (a fresh commit whose file count
 *  is over the cap, or the filter narrowing back above it after being below) — not on every
 *  keystroke that leaves the cap in the same state, which `capped` itself recomputes on. */
let lastHiddenCount: number | undefined;
watch(
  hiddenCount,
  (hidden) => {
    if (hidden > 0 && hidden !== lastHiddenCount) {
      props.actions.announce(
        t('git.fileTree.capped.announce', {
          shown: visibleRows.value.length,
          total: rows.value.length,
        }),
      );
    }
    lastHiddenCount = hidden;
  },
  { immediate: true },
);

function rowKey(row: FileTreeRow): string {
  return row.kind === 'directory' ? `dir:${row.node.path}` : `file:${row.node.path}`;
}

/** Theme `rowVariants` tree layout (`data` keeps the Git graph font reaching rows); indent comes
 *  from `rowIndent`. The focus ring is `group-focus-within` (the container carries `group`) on the
 *  roving-stop row. `data-testid="file-tree-row"` is selected by several
 *  Playwright specs. */
function rowClass(row: FileTreeRow): string {
  const selected = row.kind === 'file' && row.node.fileIndex === props.selectedFile;
  return cn(
    rowVariants({ layout: 'tree', selected, data: true }),
    nav.tabindexFor(rowKey(row)) === 0 ? 'group-focus-within:focus-ring' : '',
  );
}

/** Every selection change — a click or an arrow-key move alike — also opens the file,
 *  never pinned (the preview-tab convention governs). This is not new behaviour, only
 *  its mechanism: moving the cursor here already flipped `DetailState.mode` to
 *  `'diff'` immediately, showing the (then in-webview) diff — this is that same "select = open a
 *  preview" behaviour, now expressed as a second emit rather than a mode flip, and now something
 *  a double click/`Enter` can additionally *pin*. */
function selectRow(index: number, options: { follow?: boolean } = {}): void {
  const row = visibleRows.value[index];
  if (!row) return;
  activeKey.value = rowKey(row);
  if (row.kind === 'directory') return;
  if (options.follow === false) return;
  emit('selectFile', row.node.fileIndex);
  emit('openFile', row.node.fileIndex, false);
}

function onRowClick(index: number): void {
  const row = visibleRows.value[index];
  if (!row) return;
  if (row.kind === 'directory') {
    activeKey.value = rowKey(row);
    toggleDir(row.node.path);
    return;
  }
  selectRow(index);
}

/** The explicit "keep this" gesture — pins a real, permanent tab. Deliberately relies on
 *  the browser firing `click` first (`selectRow` above, via `onRowClick`): a double click opens
 *  the preview and then immediately re-opens the same diff pinned, and the host converts the
 *  existing preview tab into a permanent one rather than opening a second, which is why no
 *  click-delay debounce is introduced here (a debounce would add a visible lag to every single
 *  click to serve the rarer gesture). */
function onRowDblClick(index: number): void {
  const row = visibleRows.value[index];
  if (!row || row.kind === 'directory') return;
  emit('openFile', row.node.fileIndex, true);
}

function indexOfKey(key: string): number {
  return visibleRows.value.findIndex((row) => rowKey(row) === key);
}

const nav = useListNavigation<FileTreeRow>({
  container: treeEl,
  items: () => visibleRows.value,
  key: rowKey,
  activeKey,
  selectOnMove: true,
  // Arrow moves preview the file; Space acts as a click (toggle a directory, select a file).
  onSelect: (row, { reason }) => {
    const index = indexOfKey(rowKey(row));
    if (reason === 'key') onRowClick(index);
    else selectRow(index);
  },
  // G21 D13: the keyboard equivalent of a double click — pins, rather than re-emitting the same
  // navigational open `selectRow` (arrow-key nav) already fired for this row.
  onOpen: (row) => {
    if (row.kind === 'directory') toggleDir(row.node.path);
    else emit('openFile', row.node.fileIndex, true);
  },
  onMenu: (row, rect) => {
    if (row.kind === 'file') openFileMenu(rect.left, rect.bottom, row.node.change.path);
  },
  tree: {
    depth: (row) => row.depth,
    isExpanded: (row) => row.kind === 'directory' && row.expanded,
    hasChildren: (row) => row.kind === 'directory',
    expand: (row) => expandDir(row.node.path),
    collapse: (row) => collapseDir(row.node.path),
  },
});

function statusLetter(change: FileChange): string {
  return STATUS_LETTERS[change.kind];
}

function statusClass(change: FileChange): string {
  return STATUS_COLOR_CLASS[change.kind];
}

/** The row's primary leading glyph, replacing the status letter in that role; the letter
 *  itself is kept, demoted to a small secondary chip.
 *  A real per-language seti-ui icon now, not one shared codicon glyph, rendered
 *  as a CSS mask (`file-tree-icon`'s own `mask-image`) so `background-color` still drives the
 *  glyph's colour exactly as the codicon it replaces did — see `setiFileIcon.ts`'s own doc
 *  comment for why a mask rather than inline SVG/`v-html`. */
function fileIconStyle(path: string): Record<string, string> {
  const icon = setiIconFor(path);
  return { maskImage: icon.maskUrl, WebkitMaskImage: icon.maskUrl, backgroundColor: icon.color };
}

function fileTitle(change: FileChange): string {
  const rename = renameDisplay(change);
  if (rename && change.similarity !== undefined) {
    return [
      t('git.fileTree.renamed.tooltip', { from: rename.from, to: rename.to }),
      t('git.fileTree.similar.tooltip', { percent: change.similarity }),
    ].join('\n');
  }
  return change.path;
}

function copyPath(path: string): void {
  props.actions.copy(path, t('git.copy.filePath.announce'));
}

// G19 D7 / G21 D11: a right-click "Copy path" menu — the one copy-path affordance in every tree
// now (the inline button below it used to gate on is gone).
const contextMenu = useContextMenuStore();

function openFileMenu(x: number, y: number, path: string): void {
  contextMenu.openContextMenuAt(
    x,
    y,
    sectionsToMenuItems(buildFileRowMenu(props.sha !== undefined), (id) =>
      onFileMenuSelect(path, id),
    ),
  );
}

function onRowContextMenu(event: MouseEvent, row: FileTreeRow, index: number): void {
  if (row.kind !== 'file') return;
  event.preventDefault();
  // G34 D17: without this, the same `contextmenu` event also reaches `ReviewCommitRow.vue`'s own
  // handler (a file row here can sit inside an expanded review row) and a second menu — that
  // row's "Commit actions" menu — opens underneath this one from a single right-click.
  event.stopPropagation();
  selectRow(index, { follow: false });
  openFileMenu(event.clientX, event.clientY, row.node.change.path);
}

// P74 §7.4: line 1 — a tree row has no cursor, so "go to file" is the row's own job; "go to
// line" is the editor's (RepoDiffView.vue's own `repo.goToFileFromDiff` command). Announces
// either way, matching openAllChanges's own no-silent-failure posture.
async function goToFile(path: string): Promise<void> {
  const sha = props.sha;
  if (sha === undefined) return;
  try {
    const outcome = await props.actions.goToFile({ rev: sha, path, line: 1 });
    switch (outcome.kind) {
      case 'liveFile':
      case 'virtualBlob':
        props.actions.announce(t('git.fileTree.opened.announce', { path: outcome.path }));
        break;
      case 'unavailable':
        props.actions.announce(t('git.fileTree.openError.error', { path, error: outcome.reason }));
        break;
    }
  } catch (err) {
    props.actions.announce(t('git.fileTree.openError.error', { path, error: errorText(err) }));
  }
}

function onFileMenuSelect(path: string, id: string): void {
  if (id === 'copyPath') copyPath(path);
  else if (id === 'goToFile') void goToFile(path);
}

/** G14 D8: the directory portion of a flat-mode row's path, for the review sidebar's dimmed-
 *  directory-after-filename anatomy — `''` for a root-level file (nothing to show). Tree mode
 *  never calls this: the nesting itself already says which directory a row is in. */
function dirOf(path: string): string {
  const slash = path.lastIndexOf('/');
  return slash === -1 ? '' : path.slice(0, slash);
}

// Review accessors: the template calls them only behind `v-if="reviewStates"`.
function reviewAttentionFor(path: string): ReviewAttention {
  return reviewAttention(props.reviewStates?.get(path));
}
const reviewToggleTitle = (path: string): string => t(toggleTitleKey(reviewAttentionFor(path)));
const reviewCheckboxState = (path: string) => checkboxState(reviewAttentionFor(path));

const folderReview = computed(() => buildFolderReview(tree.value, props.reviewStates));

function folderState(path: string): boolean | 'indeterminate' {
  return folderReview.value.get(path)?.state ?? false;
}
function folderToggleTitle(path: string): string {
  return folderState(path) === true
    ? t('common.markFolderUnreviewed.tooltip')
    : t('common.markFolderReviewed.tooltip');
}
function onToggleFolder(path: string): void {
  const folder = folderReview.value.get(path);
  if (!folder) return;
  const change = folderToggle(folder);
  emit('toggleReviewedFolder', change.paths, change.reviewed);
}
</script>

<template>
  <div ref="rootEl" class="flex flex-col min-h-0 flex-1" data-testid="file-tree">
    <!-- A comment sitting as the root `<div>`'s own template-level sibling defeats Vue's
         single-root detection for THIS toolchain (Vue 3.5.42 /
         @vitejs/plugin-vue 6.0.8: confirmed empirically, not merely suspected), which silently
         drops every attrs-fallthrough class a caller passes — `DetailPane.vue`'s own `class` prop
         on its `<FileTree>` usage (now `flex-auto min-h-0`) never reached
         this component's root at all. -->
    <!-- P131 Part 2 §5.6: one hoisted AttributeTooltip for every row's data-kira-tip span --
         treeEl always names whichever of the two row containers below is currently mounted. -->
    <AttributeTooltip :container="treeEl" />
    <FileTreeToolbar
      :parents="parents"
      :store="store"
      :show-toolbar="showToolbar"
      :parent-index="parentIndex"
      :list-mode="listMode"
      :filter="filterInput"
      @update:parent-index="emit('update:parentIndex', $event)"
      @update:list-mode="emit('update:listMode', $event)"
      @update:filter="onFilterInput"
    />

    <div
      ref="treeEl"
      class="flex-1 min-h-0 overflow-auto group"
      :aria-label="listMode === 'tree' ? t('git.fileTree.tree.aria') : t('git.fileTree.list.aria')"
      :role="listMode === 'tree' ? 'tree' : 'listbox'"
    >
      <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter and Space come from useListNavigation on the container. -->
      <!-- biome-ignore lint/a11y/noStaticElementInteractions: role is treeitem or option by listMode. -->
      <div
        v-for="(row, index) in visibleRows"
        :key="rowKey(row)"
        data-testid="file-tree-row"
        :class="rowClass(row)"
        :data-review-state="
          reviewStates && row.kind === 'file' ? reviewAttentionFor(row.node.change.path) : undefined
        "
        :role="listMode === 'tree' ? 'treeitem' : 'option'"
        :aria-level="listMode === 'tree' ? row.depth + 1 : undefined"
        :aria-expanded="listMode === 'tree' && row.kind === 'directory' ? row.expanded : undefined"
        :aria-selected="row.kind === 'file' ? row.node.fileIndex === selectedFile : undefined"
        v-bind="nav.rowProps(rowKey(row))"
        :style="rowIndent(row.depth)"
        @click="onRowClick(index)"
        @dblclick="onRowDblClick(index)"
        @contextmenu="onRowContextMenu($event, row, index)"
      >
        <template v-if="row.kind === 'directory'">
          <Checkbox
            v-if="reviewStates"
            class="shrink-0"
            data-testid="review-folder-check"
            :model-value="folderState(row.node.path)"
            :aria-label="folderToggleTitle(row.node.path)"
            :data-kira-tip="folderToggleTitle(row.node.path)"
            @click.stop
            @update:model-value="() => onToggleFolder(row.node.path)"
          >
            <CodiconIcon
              :name="folderState(row.node.path) === 'indeterminate' ? 'dash' : 'check'"
              :size="13"
            />
          </Checkbox>
          <TreeTwisty
            :expanded="row.expanded"
            :has-children="true"
            @toggle="toggleDir(row.node.path)"
          />
          <CodiconIcon
            :name="row.expanded ? 'folder-opened' : 'folder'"
            :size="13"
            class="shrink-0 text-muted-foreground"
          />
          <span class="font-ui truncate">{{ row.node.name }}</span>
          <span class="ml-auto text-muted-foreground font-ui text-graph-sm flex gap-1">
            {{ t('common.fileCount.label', { count: row.node.fileCount }) }}
          </span>
        </template>
        <template v-else>
          <!-- Leading edge, not trailing — a trailing position shifted row to row with the
               status column and crowded the pane's scrollbar. A real Checkbox (not a
               toggle button) so the partial state gets a correct `aria-checked="mixed"` for free,
               and `.stop` (not `.prevent` -- reka's own control has no native default to prevent)
               because the server's answer is this control's only state (`ReviewFilesState.mark`
               applies `result.review`, not an optimistic local toggle). -->
          <Checkbox
            v-if="reviewStates"
            class="shrink-0"
            :class="{ 'border-warn': reviewAttentionFor(row.node.change.path) === 'changed' }"
            :model-value="reviewCheckboxState(row.node.change.path)"
            :aria-label="reviewToggleTitle(row.node.change.path)"
            :data-kira-tip="reviewToggleTitle(row.node.change.path)"
            @click.stop
            @update:model-value="() => emit('toggleReviewed', row.node.change.path)"
          >
            <CodiconIcon
              :name="
                reviewCheckboxState(row.node.change.path) === 'indeterminate' ? 'dash' : 'check'
              "
              :size="13"
            />
          </Checkbox>
          <span class="size-4 shrink-0 flex items-center justify-center" aria-hidden="true">
            <span
              class="size-3.25 mask-contain mask-no-repeat mask-center"
              :style="fileIconStyle(row.node.path)"
            ></span>
          </span>
          <span
            class="overflow-hidden text-ellipsis"
            :class="reviewStates && nameTone(reviewAttentionFor(row.node.change.path))"
            :data-kira-tip="fileTitle(row.node.change)"
          >
            <template v-if="renameDisplay(row.node.change)">
              {{ renameDisplay(row.node.change)?.from }}
              <CodiconIcon name="arrow-small-right" :size="13" />
              {{ renameDisplay(row.node.change)?.to }}
            </template>
            <template v-else>{{ row.node.name }}</template>
          </span>
          <span
            v-if="listMode === 'flat' && dirOf(row.node.path)"
            class="overflow-hidden text-ellipsis text-muted-foreground text-graph-sm"
            >{{
              dirOf(row.node.path)
            }}</span
          >
          <span class="ml-auto flex items-center gap-1 shrink-0">
            <span
              data-testid="file-tree-status"
              class="min-w-[1ch] font-data text-graph-sm leading-none shrink-0"
              :class="statusClass(row.node.change)"
              :data-kira-tip="fileTitle(row.node.change)"
              >{{
                statusLetter(row.node.change)
              }}</span
            >
          </span>
          <StatusDot
            v-if="reviewStates && reviewAttentionFor(row.node.change.path) === 'changed'"
            tone="warn"
            :data-kira-tip="t('git.review.changedSince.hint')"
            aria-hidden="true"
          />
        </template>
      </div>
    </div>

    <!-- Outside the tree/listbox: those roles may own only treeitems/options. -->
    <ShowMoreButton :hidden-count="hiddenCount" :step="capStep" @click="showMore" />
  </div>
</template>
