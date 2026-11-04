<script setup lang="ts">
import type { CommitStore } from '@kira/git-core';
import { TransportError } from '@kira/git-ipc';
/**
 * One commit in the review list. Collapsed: a disclosure triangle, subject, short sha
 * ("clicking a sha copies it"), author, relative date. Expanded: `FileTree.vue` over this row's own
 * `DetailState` (`ReviewSessionState.expand` fetches `commit.detail` on first expansion and keeps
 * it for the session — this component never fetches anything itself). `→`/`←`/`Enter` toggle
 * expansion (`ReviewView`'s `useListNavigation`); the row's own context menu is
 * copy-sha/copy-message only (`rowMenuModel.ts`'s `buildReadOnlyRowMenu`, not `buildRowMenu` —
 * this row offers no checkout/branch/tag/revert).
 *
 * Opening a file no longer renders an in-webview diff overlay at all — `FileTree`'s selection
 * calls `actions.openInEditor` directly, the same request `DetailPane.vue`'s own tree already uses.
 * `list-mode`/`filter` are props from `ReviewView.vue`'s one panel-level toolbar, not this row's
 * own `DetailState` fields — every row's `FileTree` renders no toolbar of its own
 * (`show-toolbar="false"`).
 */
import InlineState from '@theme/components/InlineState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { useEventListener } from '@vueuse/core';
import { formatAgeCompact } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import Identifier from '@workbench/components/Identifier.vue';
import TreeTwisty from '@workbench/components/TreeTwisty.vue';
import { computed, useTemplateRef } from 'vue';
import { openAllChangesAnnounced } from '../../lib/openAllChangesAnnounced.ts';
import { buildReadOnlyRowMenu } from '../../lib/rowMenuModel.ts';
import { shaLabel } from '../../lib/text.ts';
import { useSectionMenu } from '../../lib/useSectionMenu.ts';
import type { FileListMode } from '../../state/detail.ts';
import type { DetailActions } from '../../state/detailActions.ts';
import type { ReviewExpansion } from '../../state/review.ts';
import FileTree from '../FileTree.vue';

const props = defineProps<{
  sha: string;
  store: CommitStore;
  expanded: boolean;
  expansion: ReviewExpansion | undefined;
  /** P75 §1.2: the session-level row-action bundle (`ReviewSessionState.rowActions`) — reachable
   *  on a collapsed row exactly like an expanded one, unlike `expansion?.actions` (`undefined`
   *  until this row has been expanded at least once). */
  actions: DetailActions;
  /** Roving-tabindex cursor (§6.8's tree/treeitem pattern, `FileTree.vue`'s own precedent) —
   *  `ReviewView.vue` owns which row is the cursor across the whole list. */
  focused: boolean;
  /** G12 D13: `ReviewView.vue`'s one panel-level toolbar state, forwarded to this row's own
   *  `FileTree` (which renders no toolbar of its own). */
  listMode: FileListMode;
  filter: string;
}>();

const emit = defineEmits<{
  (e: 'toggle'): void;
  (e: 'focusRow'): void;
}>();

const row = computed(() => props.store.rowOfSha(props.sha));
const commit = computed(() => (row.value === -1 ? undefined : props.store.commitAt(row.value)));
const dateText = computed(() =>
  commit.value ? formatAgeCompact(commit.value.committer.timestamp) : '',
);

/** The row-action cluster (Open all changes, Open in graph) sits inside this
 *  header; a click on it is not a request to expand the row. Guarded here rather than stopped at
 *  each action. "Open in graph" is a plain button now (no more `command:` anchor VS
 *  Code's own bubble-phase link interceptor needed to see), but the guard stays — it is still
 *  correct for keeping either action's click from also toggling the row. */
function onRowClick(event: MouseEvent): void {
  if ((event.target as Element | null)?.closest('[data-testid="review-row-actions"]')) return;
  emit('focusRow');
  emit('toggle');
}

// P105 §5.2(c): the row's own `role="treeitem"` + `@keydown` already own keyboard access — this
// header is a click sub-region of it, not its own interactive element, so the listener attaches
// off-template rather than adding a second, redundant tab stop.
const headerEl = useTemplateRef<HTMLElement>('headerEl');
useEventListener(headerEl, 'click', onRowClick);

const rowMenu = useSectionMenu();

function onContextMenu(event: MouseEvent): void {
  emit('focusRow');
  rowMenu.openAt(event, buildReadOnlyRowMenu(), onMenuSelect);
}

// `props.actions` (the session-level `rowActions` bundle, see this component's own doc
// comment above) is reachable on a collapsed row exactly like an expanded one — `props.expansion`
// is `undefined` until the row has been expanded at least once, so building this menu from
// `expansion?.actions` left Copy SHA/Copy message disabled (or a no-op) on every never-expanded
// row, the same bug `openAllChanges` above was already fixed for.
function onMenuSelect(id: string): void {
  const c = commit.value;
  if (!c) return;
  if (id === 'copySha') props.actions.copy(c.sha, t('git.copy.fullSha.announce'));
  else if (id === 'copyMessage')
    props.actions.copy(c.subject, t('git.copy.commitMessage.announce'));
}

// G14 D8 row action 1 / G21 D8c: "Open all changes" — one awaited `openAllChanges` call rather
// than N unawaited `openInEditor` calls. Item 8's own remaining problem (i): the old loop fired
// every request in the same tick with a bare `void` and no `.catch`, so a run where 1 of 14 opens
// succeeded and 13 rejected looked, to the user, exactly like "it only shows one file's diff" —
// no announcement, no error surface, no partial-success reporting. This awaits the single host
// round trip and announces the real outcome either way.
//
// P75 §1.2: no expansion precondition — `props.actions` is reachable on a collapsed row (the
// old `exp?.actions.announce(...)` optional-chained through the very binding that was undefined
// on a never-expanded row, so its "expand the commit first" announcement never actually ran).
// `parentIndex` is `undefined` until the row has been expanded (nothing picked yet); the host
// default (parent 0) is exactly what an unexpanded row would have shown anyway.
async function openAllChanges(): Promise<void> {
  // G-UX D5: stopPropagation() here is now redundant with (and removed in favour of) onRowClick's
  // own review-row-actions guard above -- one rule for the whole action cluster.
  await openAllChangesAnnounced(props.actions, {
    sha: props.sha,
    parentIndex: props.expansion?.detail.parentIndex.value,
  });
}

// G14 D8 row action 3 / D10, replaced P75 §2.3: was a `command:kiraSpace.openCommitInGraph`
// anchor (a webview escape hatch) — inert in a Wails WebView with no `command:` handler at any
// layer. Now a real request the host answers locally.
async function revealInGraph(): Promise<void> {
  try {
    const { revealed } = await props.actions.revealInGraph({ sha: props.sha });
    if (!revealed) {
      props.actions.announce(t('git.review.commit.noGraph.error'));
    }
  } catch (err) {
    // A template handler — Vue's own async-error handling already stops this from
    // becoming a true unhandled rejection, but with no `app.config.errorHandler` set it never
    // reached the user either. `transport-closed` (this row's own host disconnecting mid-request)
    // is ignored outright — nothing left to announce it to once the row is gone.
    if (err instanceof TransportError && err.code === 'transport-closed') return;
    props.actions.announce(t('git.async.revealCommit.error', { error: errorText(err) }));
  }
}

// Opens the host's diff directly — no in-webview diff mode to flip into. `sha`'s
// own parentIndex is this row's current merge-parent selection, exactly what commit.detail was
// fetched against. `pinned` comes straight from FileTree's own `openFile` emit — a
// click (or arrow-key move) is `false`, a double click/`Enter` is `true`.
function onOpenFile(index: number, pinned: boolean): void {
  const exp = props.expansion;
  const file = exp?.detail.detail.value?.files[index];
  if (!exp || !file) return;
  // P108 F10: was fire-and-forget with no `.catch` — a rejection here (transport drop, a bad
  // path on the host side) became a silent unhandled rejection.
  exp.actions
    .openInEditor({
      sha: props.sha,
      path: file.path,
      originalPath: file.originalPath,
      parentIndex: exp.detail.parentIndex.value,
      pinned,
    })
    .catch((err: unknown) => {
      if (err instanceof TransportError && err.code === 'transport-closed') return;
      props.actions.announce(t('git.review.commit.openFile.error', { error: errorText(err) }));
    });
}
</script>

<template>
  <div
    v-if="commit"
    class="flex flex-col group focus-visible:focus-ring"
    role="treeitem"
    :aria-expanded="expanded"
    :tabindex="focused ? 0 : -1"
    :data-testid="`review-row-${sha}`"
    @contextmenu="onContextMenu"
  >
    <!-- A click on any file row inside review-row-body used to bubble straight up and
         collapse the very commit it was clicked inside, since nothing along the way ever called
         stopPropagation(). Only the header itself toggles the row now. -->
    <!-- Hover tint scoped to the header, not the whole row — the expanded body's own
         diff-deleted-fg text drops below 4.5:1 contrast in the dark theme against the hover tint
         (axe caught it); the header's own background (`bg-bg`, unhovered) is what the body
         already sits on, and that combination passes. -->
    <div
      data-testid="review-row-header"
      ref="headerEl"
      :class="cn(rowVariants({ layout: 'tree' }), 'py-1 px-2 min-w-0 font-ui')"
    >
      <TreeTwisty
        :expanded="expanded"
        :has-children="true"
        @toggle="
          emit('focusRow');
          emit('toggle');
        "
      />
      <!-- G14 D8 row 1: two lines — subject on its own, full-width line; author/date/sha, muted,
           below it. GitLens's own commit-node anatomy. -->
      <span class="flex flex-col gap-0.5 flex-1 min-w-0">
        <span class="truncate">{{ commit.subject }}</span>
        <span
          class="flex items-center gap-0.5 text-muted-foreground text-graph-sm whitespace-nowrap overflow-hidden"
        >
          <span class="overflow-hidden text-ellipsis">{{ commit.author.name }}</span>
          <span class="shrink-0" aria-hidden="true">·</span>
          <span class="overflow-hidden text-ellipsis">{{ dateText }}</span>
          <span class="shrink-0" aria-hidden="true">·</span>
          <!-- The clickable-sha button and its own "Copy SHA" affordance are gone — the
               sha renders as plain text; the existing copySha context-menu item
               (buildReadOnlyRowMenu) already covers this. -->
          <Identifier class="shrink-0" :value="sha" :short="shaLabel" />
        </span>
      </span>
      <!-- Inline icon actions, right-aligned — revealed on hover/focus-within (below) and
           always present for the roving-tabindex-focused row. GitLens's own row-action pattern.
           `data-testid="review-row-actions"` is also a script hook — `onRowClick`'s own
           `.closest('[data-testid="review-row-actions"]')` guard below reads it. -->
      <span
        data-testid="review-row-actions"
        :class="[
          'flex items-center gap-0.5 shrink-0',
          focused
            ? 'opacity-100'
            : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100',
        ]"
      >
        <TooltipIconButton
          icon="diff-multiple"
          :label="t('git.commit.openAllChanges.tooltip')"
          @click="openAllChanges"
        />
        <!-- A real button now — no more command: anchor a bubble-phase
             link interceptor needed to observe directly, since the reveal is a bridge request.
             onRowClick's own review-row-actions guard above still keeps this click from also
             toggling the row. -->
        <TooltipIconButton
          icon="git-commit"
          :label="t('git.review.commit.openInGraph.tooltip')"
          @click="revealInGraph"
        />
      </span>
    </div>

    <div v-if="expanded" class="min-h-30 max-h-80 flex flex-col">
      <FieldError v-if="expansion?.detail.error.value" class="p-2">
        {{ t('git.detail.loadError.error', { error: expansion.detail.error.value }) }}
      </FieldError>
      <FileTree
        v-else-if="expansion?.detail.detail.value"
        :files="expansion.detail.detail.value.files"
        :selected-file="expansion.detail.selectedFile.value"
        :list-mode="listMode"
        :filter="filter"
        :show-toolbar="false"
        :parents="expansion.detail.detail.value.parents"
        :parent-index="expansion.detail.parentIndex.value"
        :store="store"
        :actions="expansion.actions"
        @open-file="onOpenFile"
        @update:parent-index="expansion.detail.setParentIndex($event)"
      />
      <InlineState v-else kind="loading">{{ t('common.state.loading.loading') }}</InlineState>
    </div>
  </div>
</template>
