<script setup lang="ts">
/**
 * `[branch ▾]` toolbar slot. It has a five-tab structure — Branches (local + remote sub-group),
 * Tags, Stashes (stack + global-bucket sub-group), Worktrees, Stacks — over one `ToggleGroup`
 * strip (`type="single"`). `pickerModel.ts` owns the fold (filter/order/cap) for every tab; this
 * file renders the strip, the filter box and the active tab's body, and still owns every
 * ref-scoped write this file has always owned (checkout, rename, delete, the stack-navigation
 * row-menu arms) — `TagList`/`StashList`/`GlobalStashList`/`WorktreeList`/`StackList` render their
 * own tab's rows from an already filtered/ordered/capped prop, the same contract `TagList.vue` has
 * had.
 *
 * Every row also carries the ref-scoped menu (`useSectionMenu`), which is where the destructive
 * actions live — a kebab button (mouse *and* keyboard reachable) plus a plain right-click, both
 * opening the same menu.
 */
import type { StashEntry } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
import PickerTrigger from '@theme/components/PickerTrigger.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import {
  Popover,
  PopoverAnchor,
  PopoverBar,
  PopoverContent,
  PopoverTrigger,
} from '@theme/components/ui/popover';
import { type MessageKey, t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { useEventListener, useFocus } from '@vueuse/core';
import { useCappedList } from '@workbench/behaviour/cappedList';
import { useListNavigation } from '@workbench/behaviour/listNavigation';
import ShowMoreButton from '@workbench/components/ShowMoreButton.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, provide, ref, useTemplateRef, watch } from 'vue';
import { PICKER_TAB_ICONS } from '../icons/index.ts';
import {
  filterPickerInput,
  orderTab,
  type PickerInput,
  type PickerList,
  type PickerTab,
  pickerRowIds,
} from '../lib/pickerModel.ts';
import { PICKER_FOCUS_ROW } from '../lib/pickerNav.ts';
import { REF_LIST_SECTION_CAP } from '../lib/refListModel.ts';
import { remoteNamesFrom } from '../lib/rowMenuModel.ts';
import { shaLabel } from '../lib/text.ts';
import { useFieldFocus } from '../lib/useFieldFocus.ts';
import type { ChangeRequestState } from '../state/changeRequests.ts';
import type { OpsState } from '../state/ops.ts';
import type { RefsState } from '../state/refs.ts';
import type { StackState } from '../state/stack.ts';
import type { StashState } from '../state/stash.ts';
import type { WorktreeCreateSeed, WorktreeState } from '../state/worktrees.ts';
import GlobalStashList from './GlobalStashList.vue';
import RefsPopoverBranches from './RefsPopoverBranches.vue';
import StackList from './StackList.vue';
import StashList from './StashList.vue';
import TagList from './TagList.vue';
import WorktreeList from './WorktreeList.vue';

const props = defineProps<{
  refs: RefsState;
  ops: OpsState;
  stash: StashState;
  worktrees: WorktreeState;
  /** G26 D3 — see `StackList.vue`'s own doc comment. */
  stack: StackState;
  /** G24 D9's own branch-tip badge — optional so a caller with nothing to show yet gets a plain,
   *  badge-free picker (mirrors `CommitGrid.vue`'s own `pr` prop). */
  changeRequests?: ChangeRequestState;
  /** P74 §3.3: `AppToolbar.vue`'s own `openChangeRequest` — threaded down rather than reimplemented
   *  here or in `StackList.vue`. */
  openChangeRequest: (number: number) => void;
}>();

/** OQ3: bubbled straight through from `StashList.vue`'s own emit — see that component's own doc
 *  comment on why the branch-mode dialog itself is owned by `App.vue`, not here. */
const emit = defineEmits<{
  (e: 'branchFromStash', entry: StashEntry): void;
  /** G28 D13: bubbled to `App.vue`, which owns `StashDialog.vue`'s save-to-global-stash mode —
   *  same "this component has nowhere of its own to render a dialog into" shape
   *  `branchFromStash` already follows. */
  (e: 'saveGlobalStash'): void;
  /** G28 D13: the same save mode, opened with THIS entry pre-selected as its source
   *  (`StashList.vue`'s own "Save to global stash…" row action). */
  (e: 'saveEntryToGlobalStash', entry: StashEntry): void;
  (e: 'switchWorktree', path: string): void;
  /** P76 §9.3: widened to carry an optional seed — `undefined` from `WorktreeList.vue`'s own
   *  create button, a seed from this picker's own "Create worktree here…" row action. */
  (e: 'createWorktree', seed?: WorktreeCreateSeed): void;
  (e: 'openRestackDialog', branch: string): void;
  (e: 'openSetStackParentDialog', branch: string): void;
}>();

const TAB_LABELS: Readonly<Record<PickerTab, MessageKey>> = {
  branches: 'common.module.branches.label',
  tags: 'git.tag.list.label',
  stashes: 'git.stash.list.label',
  worktrees: 'common.module.worktrees.label',
  stacks: 'git.stack.list.label',
};

const TAB_FILTER_PLACEHOLDERS: Readonly<Record<PickerTab, MessageKey>> = {
  branches: 'git.picker.filterBranches.placeholder',
  tags: 'git.picker.filterTags.placeholder',
  stashes: 'git.picker.filterStashes.placeholder',
  worktrees: 'git.picker.filterWorktrees.placeholder',
  stacks: 'git.picker.filterStacks.placeholder',
};

const isOpen = ref(false);
const triggerEl = useTemplateRef<{ $el: HTMLElement }>('triggerEl');
const { focused: triggerFocused } = useFocus(computed(() => triggerEl.value?.$el));
function focusTrigger(): void {
  triggerFocused.value = true;
}
const filterEl = useTemplateRef<{ $el: Element }>('filterEl');
const focusFilter = useFieldFocus(filterEl);
const filter = ref('');
const activeTab = ref<PickerTab>('branches');
// P77 §7.3: the roving-tabindex list's own "current" row — `undefined` until the user presses an
// arrow key, at which point `activeRowId` below still resolves it (falls back to the first
// enabled row), so a fresh tab/query always has exactly one tabbable row.
const focusedRowId = ref<string | null>(null);

const triggerLabel = computed(() => {
  const head = props.refs.head.value;
  if (!head) return '…';
  if (head.kind === 'branch') return head.name;
  if (head.kind === 'unborn') return t('git.picker.unborn.label', { name: head.name });
  return shaLabel(head.sha);
});

const pickerInput = computed<PickerInput>(() => ({
  branches: props.refs.branches.value,
  remoteBranches: props.refs.remoteBranches.value,
  tags: props.refs.tags.value,
  stashes: props.stash.entries.value,
  globalStashes: props.stash.globalEntries.value,
  worktrees: props.worktrees.entries.value,
  stacks: props.stack.stacks.value,
  orphans: props.stack.orphans.value,
}));

// Split into two computeds so a tab switch (only ever changing `activeTab`) never re-runs the
// expensive filter fold over every list — `filteredPicker` depends only on `(pickerInput, filter)`,
// `ordered` only adds the cheap per-tab ordering pass on top. See `pickerModel.ts`'s own doc
// comment on `filterPickerInput`.
const filteredPicker = computed(() => filterPickerInput(pickerInput.value, filter.value));
const ordered = computed(() => orderTab(filteredPicker.value, activeTab.value));

// P77 §6.3: one list's own "Show 50 more" cap for the current panel-open — reset on close and on a
// filter change (a new query is a new list, §6.3's own words), never persisted.
const caps = {
  branchesLocal: useCappedList(() => ordered.value.branchesLocal, REF_LIST_SECTION_CAP),
  branchesRemote: useCappedList(() => ordered.value.branchesRemote, REF_LIST_SECTION_CAP),
  tags: useCappedList(() => ordered.value.tags, REF_LIST_SECTION_CAP),
  stashStack: useCappedList(() => ordered.value.stashStack, REF_LIST_SECTION_CAP),
  stashGlobal: useCappedList(() => ordered.value.stashGlobal, REF_LIST_SECTION_CAP),
  worktrees: useCappedList(() => ordered.value.worktrees, REF_LIST_SECTION_CAP),
  stacks: useCappedList(() => ordered.value.stacks, REF_LIST_SECTION_CAP),
  orphans: useCappedList(() => ordered.value.orphans, REF_LIST_SECTION_CAP),
};

function resetCaps(): void {
  for (const cap of Object.values(caps)) cap.reset();
}

function sectionOf<T>(cap: {
  visible: { value: readonly T[] };
  hiddenCount: { value: number };
}): PickerList<T> {
  return { visible: cap.visible.value, hiddenCount: cap.hiddenCount.value };
}

const model = computed(() => {
  const lists = {
    branchesLocal: sectionOf(caps.branchesLocal),
    branchesRemote: sectionOf(caps.branchesRemote),
    tags: sectionOf(caps.tags),
    stashStack: sectionOf(caps.stashStack),
    stashGlobal: sectionOf(caps.stashGlobal),
    worktrees: sectionOf(caps.worktrees),
    stacks: sectionOf(caps.stacks),
    orphans: sectionOf(caps.orphans),
  };
  return {
    ...lists,
    counts: filteredPicker.value.counts,
    rowIds: pickerRowIds(activeTab.value, {
      branchesLocal: lists.branchesLocal.visible,
      branchesRemote: lists.branchesRemote.visible,
      tags: lists.tags.visible,
      stashStack: lists.stashStack.visible,
      stashGlobal: lists.stashGlobal.visible,
      worktrees: lists.worktrees.visible,
      stacks: lists.stacks.visible,
      orphans: lists.orphans.visible,
    }),
  };
});

function showMoreStacks(): void {
  caps.stacks.showMore();
  caps.orphans.showMore();
}

watch(filter, resetCaps);

// ---------------------------------------------------------------------------------------
// Roving focus over the active tab's own rows through `useListNavigation`. The rows live in
// this picker's child lists, so the roving stop is tracked from `focusin` on the shared container.
// `branch-row`/`branch-row-main` stay as bare query hooks for `closest()`/`querySelector()`.
// ---------------------------------------------------------------------------------------
const rowsScrollEl = ref<HTMLElement | null>(null);
type PickerRow = (typeof model.value.rowIds)[number];

const activeRowId = computed<string | undefined>(() => {
  const rows = model.value.rowIds;
  if (focusedRowId.value !== null && rows.some((r) => r.id === focusedRowId.value)) {
    return focusedRowId.value;
  }
  return rows.find((r) => !r.disabled)?.id;
});

function rowEl(id: string): HTMLElement | null {
  return (
    rowsScrollEl.value?.querySelector<HTMLElement>(`[data-row-id="${CSS.escape(id)}"]`) ?? null
  );
}

const nav = useListNavigation<PickerRow>({
  container: rowsScrollEl,
  items: () => model.value.rowIds,
  key: (row) => row.id,
  isDisabled: (row) => row.disabled,
  activeKey: focusedRowId,
  rowSelector: (key) => `[data-row-id="${CSS.escape(key)}"]`,
  // Worktree/stack rows have no main button, so Enter does nothing there.
  onOpen: (row) =>
    rowEl(row.id)?.querySelector<HTMLElement>('[data-testid="branch-row-main"]')?.click(),
  onMenu: (row) =>
    rowEl(row.id)?.querySelector<HTMLElement>('[data-testid="row-actions"]')?.click(),
  onExitTop: () => focusFilter(),
});

useEventListener(rowsScrollEl, 'focusin', (event: FocusEvent) => {
  const id = (event.target as HTMLElement).closest<HTMLElement>('[data-row-id]')?.dataset.rowId;
  if (id !== undefined) focusedRowId.value = id;
});

provide(PICKER_FOCUS_ROW, (rowId: string) => void nav.focusKey(rowId));

const contextMenuStore = useContextMenuStore();

/** Right-click: the row becomes the roving stop and takes focus (B1), then the menu opens. */
function openRowContextMenu(event: MouseEvent, items: MenuItem[], rowId: string): void {
  const item = model.value.rowIds.find((r) => r.id === rowId);
  if (item === undefined) contextMenuStore.openContextMenu(event, items);
  else nav.contextMenu(item, event, items);
}

/** §7.2: `ArrowDown` from the filter box moves into the list. */
function onFilterDown(event: KeyboardEvent): void {
  const first = model.value.rowIds.find((row) => !row.disabled);
  if (first === undefined) return;
  event.preventDefault();
  void nav.focusKey(first.id);
}

// P131 Part 2 §5.2: KuiSegmentedOption goes with KuiSegmented -- this is the same shape, inlined
// as a local type instead of importing one.
interface PickerTabOption {
  readonly id: PickerTab;
  readonly icon: string;
  readonly label: string;
  readonly badge: number;
}

const tabOptions = computed<readonly PickerTabOption[]>(() => [
  {
    id: 'branches',
    icon: PICKER_TAB_ICONS.branches,
    label: t(TAB_LABELS.branches),
    badge: model.value.counts.branches,
  },
  {
    id: 'tags',
    icon: PICKER_TAB_ICONS.tags,
    label: t(TAB_LABELS.tags),
    badge: model.value.counts.tags,
  },
  {
    id: 'stashes',
    icon: PICKER_TAB_ICONS.stashes,
    label: t(TAB_LABELS.stashes),
    badge: model.value.counts.stashes,
  },
  {
    id: 'worktrees',
    icon: PICKER_TAB_ICONS.worktrees,
    label: t(TAB_LABELS.worktrees),
    badge: model.value.counts.worktrees,
  },
  {
    id: 'stacks',
    icon: PICKER_TAB_ICONS.stacks,
    label: t(TAB_LABELS.stacks),
    badge: model.value.counts.stacks,
  },
]);
const tabItems = computed(() =>
  tabOptions.value.map((tab) => ({
    value: tab.id,
    label: tab.label,
    icon: tab.icon,
    count: tab.badge,
    tooltip: tab.label,
  })),
);

const knownRemotes = computed(() =>
  remoteNamesFrom(props.refs.remoteBranches.value.map((row) => row.shortName)),
);

// close()'s resets run from the Popover's own @update:open(false), reached by trigger click,
// Escape, outside click or an explicit close() call.
function close(): void {
  isOpen.value = false;
  filter.value = '';
  resetCaps();
}

// Forwarded so App.vue's palette dispatcher can open this panel exactly the way
// clicking its own trigger does — the picker is the only place the UI names a ref/stash/worktree/
// stack to act on, so every such palette command reaches one of `openBranchPicker`/`openTagPicker`/
// `openStashPicker`/`openWorktreePicker`/`openStackPicker`. `tab` is optional so the click-trigger
// path (which always wants whatever tab was last open) is unchanged.
function open(tab?: PickerTab): void {
  if (tab !== undefined) activeTab.value = tab;
  isOpen.value = true;
}
defineExpose({ open });

/** The Popover's own `@update:open` -- fired for every dismissal (trigger click, Escape, outside
 *  click) and every trigger-click open. `true` just tracks the boolean (the palette route's own
 *  `open()` already set it directly); the filter-focus-on-open side effect lives in
 *  `onOpenAutoFocus` below, so it fires for both routes alike. */
function onOpenChange(value: boolean): void {
  if (value) isOpen.value = true;
  else close();
}

/** §7.2: "filter focused on open" for both the click-trigger and palette (`open()`) routes --
 *  `preventDefault()` skips reka's own default (the panel's first focusable element) so the
 *  filter input wins regardless of where it sits in the panel. */
function onOpenAutoFocus(e: Event): void {
  e.preventDefault();
  focusFilter();
}

/** reka's own close-auto-focus fires after the panel's exit animation, which could steal
 *  focus back from a dialog `runCheckout` opens in the meantime (`closeForCheckout` below already
 *  moves focus to the trigger synchronously, before that animation even starts). Suppressed for
 *  exactly that one call, not for every close. */
let suppressCloseAutoFocus = false;
function onCloseAutoFocus(e: Event): void {
  if (!suppressCloseAutoFocus) return;
  e.preventDefault();
  suppressCloseAutoFocus = false;
}

/** `close()` unmounts the whole panel, including whatever row button the click just
 *  focused — by the time `runCheckout` might open `CheckoutDialog.vue`, that button is gone and
 *  a dialog's own focus-return capture would land on nothing (the browser's own fallback,
 *  `<body>`). Moving focus to the trigger *first*, synchronously — a stable control that survives
 *  the panel's own close — gives that capture something real to return to. `suppressCloseAutoFocus`
 *  stops reka's own close-auto-focus (which fires after the exit animation) from firing later and
 *  stealing focus back from whatever dialog `runCheckout` opened in the meantime. */
function closeForCheckout(): void {
  focusTrigger();
  suppressCloseAutoFocus = true;
  close();
}

// Opening the picker is one of the few user acts allowed to touch the network at
// all — warms every visible branch's own PR record in one go. "visible" used to mean the repo's
// FULL branch list, not what `model` (above) renders — `branch.resolveChangeRequest` answers a
// branch WITH an open PR from its already-cached bulk snapshot at no extra cost, but one WITHOUT
// falls through to its own per-branch host call, so a repo with hundreds of branches fanned out
// hundreds of host calls from one picker open, easily arming the host rate-limit breaker. Scoped to
// the Branches tab's own visible rows (each capped at `REF_LIST_SECTION_CAP`) and keyed off that
// computed itself, not just `isOpen`, so typing a filter that surfaces a branch outside the initial
// page still gets it warmed. A picker opened on a non-Branches tab now warms nothing at all, since
// `model.branchesLocal`/`branchesRemote` are only ever materialised for the active tab.
const visibleBranchNames = computed<readonly string[]>(() => {
  if (!isOpen.value) return [];
  return [
    ...model.value.branchesLocal.visible.map((r) => r.shortName),
    ...model.value.branchesRemote.visible.map((r) => r.shortName),
  ];
});
watch(visibleBranchNames, (names) => {
  if (names.length === 0 || !props.changeRequests) return;
  void props.changeRequests.ensureSnapshot(names);
});
</script>

<template>
  <div class="relative">
    <Popover :open="isOpen" modal @update:open="onOpenChange">
      <PopoverAnchor as-child>
        <TextTooltip :text="triggerLabel">
          <PopoverTrigger as-child>
            <PickerTrigger
              data-testid="branch-trigger"
              ref="triggerEl"
              size="kira"
              class="max-w-50"
              :label="triggerLabel"
            >
              <template #leading><CodiconIcon name="git-branch" :size="13" /></template>
            </PickerTrigger>
          </PopoverTrigger>
        </TextTooltip>
      </PopoverAnchor>

      <PopoverContent
        align="start"
        flush
        class="w-96"
        :aria-label="t('git.picker.region.aria', { tab: t(TAB_LABELS[activeTab]) })"
        @open-auto-focus="onOpenAutoFocus"
        @close-auto-focus="onCloseAutoFocus"
      >
        <div
          class="flex flex-col min-h-0 max-h-[min(520px,var(--reka-popover-content-available-height))]"
        >
          <PopoverBar>
            <SecondaryTabs
              data-testid="branch-tabs"
              class="w-full"
              :aria-label="t('git.picker.sections.aria')"
              :model-value="activeTab"
              :items="tabItems"
              @update:model-value="(v) => (activeTab = v as PickerTab)"
            >
              <template #item="{ item }">
                <CodiconIcon :name="item.icon ?? ''" :size="13" />
                <span class="sr-only">{{ item.label }}</span>
                <span :class="rowMeta" data-testid="picker-tab-badge">{{ item.count }}</span>
              </template>
            </SecondaryTabs>
          </PopoverBar>

          <PopoverBar>
            <SearchField
              ref="filterEl"
              v-model="filter"
              :placeholder="t(TAB_FILTER_PLACEHOLDERS[activeTab])"
              :aria-label="t(TAB_FILTER_PLACEHOLDERS[activeTab])"
              @keydown.down="onFilterDown"
            />
          </PopoverBar>

          <div
            ref="rowsScrollEl"
            class="overflow-y-auto min-h-0"
            :aria-label="t(TAB_LABELS[activeTab])"
          >
            <AttributeTooltip :container="rowsScrollEl" />
            <RefsPopoverBranches
              v-if="activeTab === 'branches'"
              :refs="refs"
              :ops="ops"
              :stack="stack"
              :change-requests="changeRequests"
              :open-change-request="openChangeRequest"
              :local-section="model.branchesLocal"
              :remote-section="model.branchesRemote"
              :active-row-id="activeRowId"
              :close-for-checkout="closeForCheckout"
              :open-context-menu="openRowContextMenu"
              @create-worktree="(seed) => emit('createWorktree', seed)"
              @open-restack-dialog="(branch) => emit('openRestackDialog', branch)"
              @open-set-stack-parent-dialog="(branch) => emit('openSetStackParentDialog', branch)"
            >
              <template #local-more>
                <ShowMoreButton
                  :step="REF_LIST_SECTION_CAP"
                  :hidden-count="model.branchesLocal.hiddenCount"
                  @click="caps.branchesLocal.showMore()"
                />
              </template>
              <template #remote-more>
                <ShowMoreButton
                  :step="REF_LIST_SECTION_CAP"
                  :hidden-count="model.branchesRemote.hiddenCount"
                  @click="caps.branchesRemote.showMore()"
                />
              </template>
            </RefsPopoverBranches>

            <template v-else-if="activeTab === 'tags'">
              <TagList
                :section="model.tags"
                :ops="ops"
                :known-remotes="knownRemotes"
                :in-progress="ops.statusSummary.value?.inProgress ?? null"
                :focused-row-id="activeRowId"
                @checkedOut="close"
              />
              <ShowMoreButton
                :step="REF_LIST_SECTION_CAP"
                :hidden-count="model.tags.hiddenCount"
                @click="caps.tags.showMore()"
              />
            </template>

            <template v-else-if="activeTab === 'stashes'">
              <StashList
                :section="model.stashStack"
                :stash="stash"
                :ops="ops"
                :in-progress="ops.statusSummary.value?.inProgress ?? null"
                :current-branch="refs.currentBranchName.value ?? null"
                :focused-row-id="activeRowId"
                @branchFromStash="(entry) => emit('branchFromStash', entry)"
                @saveEntryToGlobalStash="(entry) => emit('saveEntryToGlobalStash', entry)"
                @selected="closeForCheckout"
              />
              <ShowMoreButton
                :step="REF_LIST_SECTION_CAP"
                :hidden-count="model.stashStack.hiddenCount"
                @click="caps.stashStack.showMore()"
              />

              <GlobalStashList
                :section="model.stashGlobal"
                :stash="stash"
                :ops="ops"
                :in-progress="ops.statusSummary.value?.inProgress ?? null"
                :current-branch="refs.currentBranchName.value ?? null"
                :focused-row-id="activeRowId"
                @branchFromStash="(entry) => emit('branchFromStash', entry)"
                @saveGlobalStash="emit('saveGlobalStash')"
                @selected="closeForCheckout"
              />
              <ShowMoreButton
                :step="REF_LIST_SECTION_CAP"
                :hidden-count="model.stashGlobal.hiddenCount"
                @click="caps.stashGlobal.showMore()"
              />
            </template>

            <template v-else-if="activeTab === 'worktrees'">
              <WorktreeList
                :section="model.worktrees"
                :worktrees="worktrees"
                :ops="ops"
                :focused-row-id="activeRowId"
                @switchWorktree="(path) => emit('switchWorktree', path)"
                @createWorktree="emit('createWorktree')"
              />
              <ShowMoreButton
                :step="REF_LIST_SECTION_CAP"
                :hidden-count="model.worktrees.hiddenCount"
                @click="caps.worktrees.showMore()"
              />
            </template>

            <template v-else-if="activeTab === 'stacks'">
              <StackList
                :stacks="model.stacks"
                :orphans="model.orphans"
                :ops="ops"
                :change-requests="changeRequests"
                :open-change-request="openChangeRequest"
                :focused-row-id="activeRowId"
                @openRestackDialog="(branch) => emit('openRestackDialog', branch)"
                @openSetParentDialog="(branch) => emit('openSetStackParentDialog', branch)"
              />
              <ShowMoreButton
                :step="REF_LIST_SECTION_CAP"
                :hidden-count="model.stacks.hiddenCount + model.orphans.hiddenCount"
                @click="showMoreStacks"
              />
            </template>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  </div>
</template>
