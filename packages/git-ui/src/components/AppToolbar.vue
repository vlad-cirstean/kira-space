<script setup lang="ts">
/**
 * The toolbar: `[repo ▾] [branch ▾] │ ⟳ │ Fetch Pull Push │ Stash ▾ │ ⚙`. The second group — the
 * branch/tag picker — and the undo affordance are there since both need the ref list and op
 * executor. The trailing `⚙` is a settings gear: the gear opens
 * `App.vue`'s own `RepoSettingsDialog.vue`, this toolbar owning no dialog state of its own
 * (`openRepoSettings` emit), the same shape `stashChanges` already follows.
 *
 * `SearchBox.vue` is out of this row entirely — a row of its own, below the toolbar,
 * toggled rather than always rendered (`App.vue` owns it now, alongside `AppToolbar`, not this
 * component).
 *
 * Chrome: `ViewToolbar` (the app's bar height, border and padding), Kira's radius and shadow tiers.
 *
 * There is no `remotes.list` endpoint (per `rowMenuModel.ts`'s own
 * `remoteNamesFrom` doc comment) and remote *management* is out of scope entirely, so this
 * toolbar reads remote names from whatever remote-tracking branches already
 * loaded and targets the current branch's upstream remote, else `origin`, else the first name —
 * `defaultRemote` below. Fetch/Pull/Push are absent (not disabled) when no remote is
 * known at all: there is nothing to name in the tooltip and no useful default to pick.
 */
import type { StashEntry } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import SplitButton from '@theme/components/SplitButton.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { DropdownMenuContent } from '@theme/components/ui/dropdown-menu';
import { type MessageKey, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import MenuItems from '@workbench/components/MenuItems.vue';
import ToolbarSeparator from '@workbench/components/ToolbarSeparator.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref } from 'vue';
import { type MenuSection, sectionsToMenuItems } from '../lib/menuModel.ts';
import type { PickerTab } from '../lib/pickerModel.ts';
import { defaultRemoteFor, remoteNamesFrom } from '../lib/rowMenuModel.ts';
import type { AsyncFailure } from '../state/asyncFailures.ts';
import type { ChangeRequestState } from '../state/changeRequests.ts';
import type { DetailActions } from '../state/detailActions.ts';
import type { GraphViewState } from '../state/graphView.ts';
import { composeAutoFetchStoppedAnnouncement } from '../state/liveAnnouncements.ts';
import type { OpsState } from '../state/ops.ts';
import type { RefsState } from '../state/refs.ts';
import type { RepoState } from '../state/repo.ts';
import type { StackState } from '../state/stack.ts';
import type { StashState } from '../state/stash.ts';
import type { WorktreeCreateSeed, WorktreeState } from '../state/worktrees.ts';
import PullStrategyPicker from './PullStrategyPicker.vue';
import RefreshButton from './RefreshButton.vue';
// Plain (not `import type`) imports, for two different reasons. RefsPopover: vue-tsc needs the
// real import to infer the template's inline @branchFromStash handler's parameter type from
// RefsPopover's own emits declaration — a type-only import here breaks that inference (TS7006).
// PullStrategyPicker and RefreshButton: a .vue default export is a *value* — the component object
// the template instantiates. `import type` erases it, and Vue then renders the tag as an unknown
// element with nothing inside it. The script's only reference to RefreshButton is
// `InstanceType<typeof …>`, so biome's useImportType cannot tell (it is off for `**/*.vue` in
// `biome.json` for exactly this class of false positive); the template is the real caller.
import RefsPopover from './RefsPopover.vue';
import UndoButton from './UndoButton.vue';

const props = defineProps<{
  graphView: GraphViewState;
  repoState: RepoState;
  refsState: RefsState;
  opsState: OpsState;
  stashState: StashState;
  worktreeState: WorktreeState;
  /** G26 D3 — see `StackList.vue`'s own doc comment. */
  stackState: StackState;
  actions: DetailActions | undefined;
  /** G24 D9: `RefsPopover.vue`'s own `#123` branch-tip badge source — optional, mirrors every
   *  other G24 prop threaded through this toolbar's own children. */
  changeRequestState?: ChangeRequestState;
  /** Whether `App.vue`'s own search row is currently open — drives the toggle button's `active`
   *  state, the same `active` = "revealed" convention `ReviewView.vue`'s own filter toggle uses. */
  searchOpen: boolean;
  /** P93 §4.4/§7: whether every non-checked-out branch group collapses by default — drives the
   *  collapse-toggle button's own `active` state, same convention as `searchOpen` above. */
  collapseBranches: boolean;
  /** Routes a rejected refresh to the shared live region (`App.vue`'s `reportAsyncError`). */
  reportError: (err: unknown, failure: AsyncFailure) => void;
}>();
const emit = defineEmits<{
  /** `docs/plans/P9.md` W14: opens `StashDialog.vue`'s create mode — owned by `App.vue`, exactly
   *  like `createBranchHere`/`createTagHere`'s own dialog state, since this button has no
   *  pre-flight endpoint of its own to preview first (the dialog IS the confirm step). */
  (event: 'stashChanges'): void;
  /** Forwarded straight from `RefsPopover.vue`'s own emit — see `StashList.vue`'s doc comment. */
  (event: 'branchFromStash', entry: StashEntry): void;
  /** G28 D13: forwarded straight from `RefsPopover.vue`'s own emits — see
   *  `GlobalStashList.vue`'s/`StashList.vue`'s own doc comments. */
  (event: 'saveGlobalStash'): void;
  (event: 'saveEntryToGlobalStash', entry: StashEntry): void;
  /** G25: forwarded straight from `RefsPopover.vue` -> `WorktreeList.vue`'s own emits — see
   *  `WorktreeList.vue`'s own doc comment on why this toolbar does not act on them itself. */
  (event: 'switchWorktree', path: string): void;
  (event: 'createWorktree', seed?: WorktreeCreateSeed): void;
  /** G26: forwarded straight from `RefsPopover.vue` -> `StackList.vue`'s own emits — `App.vue`
   *  owns `StackDialog.vue`'s actual open state, the same "toolbar owns no dialog state itself"
   *  shape every other dialog-opening emit above already follows. */
  (event: 'openRestackDialog', branch: string): void;
  (event: 'openSetStackParentDialog', branch: string): void;
  /** G18 D13: the settings gear (`⚙`) this toolbar's own ascii layout has named since §6.2 but no
   *  phase implemented until now — opens `App.vue`'s own `RepoSettingsDialog.vue`, the same
   *  "toolbar owns no dialog state itself" shape `stashChanges` above already follows. */
  (event: 'openRepoSettings'): void;
  /** Toggles `App.vue`'s own search row (`toggleSearchRow`) — the toolbar owns no search state of
   *  its own, the same "emit, don't own" shape every other dialog/panel-toggling emit above
   *  already follows. */
  (event: 'toggleSearch'): void;
  /** P93 §4.4/§7: toggles `App.vue`'s own `collapseBranches` preference — the toolbar owns no
   *  collapse state of its own, same "emit, don't own" shape as `toggleSearch` above. */
  (event: 'toggleCollapseBranches'): void;
}>();

function copy(text: string, message: string): void {
  props.actions?.copy(text, message);
}

// P74 §3.3: RefsPopover.vue's own branch-row badge and StackList.vue's own row badge each open
// a PR the same way CommitMeta.vue's facts-row icon/change request row do — one action, threaded
// down rather than reimplemented at either call site.
function openChangeRequest(number: number): void {
  void props.actions?.openChangeRequest({ number });
}

const refreshButtonRef = ref<InstanceType<typeof RefreshButton> | null>(null);
const refsPopoverRef = ref<InstanceType<typeof RefsPopover> | null>(null);
const pullStrategyPickerRef = ref<InstanceType<typeof PullStrategyPicker> | null>(null);

// ---------------------------------------------------------------------------------------
// P8 W17: fetch/pull/push
// ---------------------------------------------------------------------------------------

const defaultRemote = computed(() =>
  defaultRemoteFor(
    remoteNamesFrom(props.refsState.remoteBranches.value.map((row) => row.shortName)),
    props.refsState.branches.value.find((row) => row.isHead)?.upstream,
  ),
);
const currentBranch = computed(() => props.refsState.currentBranchName.value);
const hasRemote = computed(() => defaultRemote.value !== undefined);
/** A conflicting merge/rebase (or anything else in P6's in-progress banner) blocks Pull/Push —
 *  both would touch a worktree or history that is mid-operation — but never blocks Fetch, which
 *  touches neither (§4.3/D50's own distinction, read forward into the toolbar's own gate). */
const inConflict = computed(() => props.opsState.statusSummary.value?.inProgress !== null);
const remoteBusy = computed(() => props.opsState.activeRemoteOp.value !== undefined);

const fetchDisabled = computed(() => !hasRemote.value || props.opsState.busy.value);
const pushPullDisabled = computed(
  () =>
    !hasRemote.value ||
    currentBranch.value === undefined ||
    props.opsState.busy.value ||
    inConflict.value,
);

const autoFetchStopped = computed(() => props.opsState.autoFetch.value);
const autoFetchTooltip = computed(() => {
  const stopped = autoFetchStopped.value;
  if (stopped === null) return '';
  return composeAutoFetchStoppedAnnouncement(stopped.kind);
});

async function onAutoFetchMarkerClick(): Promise<void> {
  if (hasRemote.value && !fetchDisabled.value) await doFetch();
}

const isForcePushMenuOpen = ref(false);

// One item through `sectionsToMenuItems`/`MenuItems`. `danger: true` renders the error tone.
const pushMenuSections = computed<readonly MenuSection[]>(() => [
  {
    items: [
      {
        id: 'force-push-trigger',
        label: t('git.menu.forcePush.label'),
        disabled: false,
        disabledReason: undefined,
        danger: true,
      },
    ],
  },
]);

const pushMenuItems = computed(() =>
  sectionsToMenuItems(pushMenuSections.value, (id) => {
    if (id === 'force-push-trigger') return doForcePush();
  }),
);

async function doFetch(): Promise<void> {
  const remote = defaultRemote.value;
  if (remote === undefined) return;
  await props.opsState.runFetch(remote);
}

async function doPush(): Promise<void> {
  const remote = defaultRemote.value;
  const branch = currentBranch.value;
  if (remote === undefined || branch === undefined) return;
  await props.opsState.runPush(remote, branch);
}

async function doForcePush(): Promise<void> {
  isForcePushMenuOpen.value = false;
  const remote = defaultRemote.value;
  const branch = currentBranch.value;
  if (remote === undefined || branch === undefined) return;
  await props.opsState.runForcePush(remote, branch);
}

const remoteOpLabel: Record<string, MessageKey> = {
  fetch: 'common.fetching.loading',
  pull: 'git.toolbar.pulling.loading',
  push: 'git.toolbar.pushing.loading',
  forcePush: 'git.toolbar.forcePushing.loading',
  deleteRemoteBranch: 'git.toolbar.deletingRemoteBranch.loading',
};

const progressText = computed(() => {
  const kind = props.opsState.activeRemoteOp.value;
  if (kind === undefined) return '';
  const progress = props.opsState.remoteProgress.value;
  const label = remoteOpLabel[kind];
  if (!progress) return label === undefined ? '' : t(label);
  const phase =
    progress.percent === undefined
      ? progress.phase
      : t('common.progressPercent.label', { label: progress.phase, percent: progress.percent });
  return progress.remote ? t('git.toolbar.progressRemote.status', { phase }) : phase;
});

/** The cancellability table, read into the toolbar's own affordance: only fetch and pull's own
 *  fetch phase are killable, so the button is disabled-with-reason for the other three kinds
 *  rather than hidden — clicking it while, say, a push is in flight is a legitimate thing to try,
 *  and `remote.cancel` answers honestly (`cancelled: false`) either way (the "cancel is
 *  refused mid-push" criterion). */
const cancelDisabledReason = computed(() => {
  switch (props.opsState.activeRemoteOp.value) {
    case 'fetch':
    case 'pull':
      return undefined;
    case 'push':
      return t('git.toolbar.cancelPush.hint');
    case 'forcePush':
      return t('git.toolbar.cancelForcePush.hint');
    case 'deleteRemoteBranch':
      return t('git.toolbar.cancelDeleteRemote.hint');
    default:
      return t('git.toolbar.nothingRunning.hint');
  }
});
const cancellable = computed(() => cancelDisabledReason.value === undefined);

async function doCancel(): Promise<void> {
  await props.opsState.cancelRemote();
}

// Forwarded so App.vue's palette dispatcher can drive the same affordances a click
// already does — one implementation, reached from two inputs, exactly like `refresh` above. Each
// is a one-line delegation to a handler this file already has, or to a nested component's own
// `defineExpose`.
defineExpose({
  refresh: () => refreshButtonRef.value?.refresh(),
  openBranchPicker: (tab?: PickerTab) => refsPopoverRef.value?.open(tab),
  fetch: doFetch,
  pull: () => pullStrategyPickerRef.value?.run(),
  push: doPush,
  forcePush: doForcePush,
  cancelRemote: doCancel,
});

/** §7.6/W14: "enabled from `StatusSummary`'s dirty flag" — `isClean` rather than a `dirtyPaths`
 *  length check, since the summary is the same object every other toolbar gate already reads. */
const stashDisabled = computed(
  () => (props.opsState.statusSummary.value?.isClean ?? true) || props.opsState.busy.value,
);
</script>

<template>
  <!-- (axe `aria-allowed-role`): `role="toolbar"` is not among the roles the ARIA spec
       allows overriding a `<header>`'s own implicit "banner" role with — a plain `<div>` carries
       no implicit role of its own to conflict with the explicit one, which is all this element
       ever wanted (the layout, not a page banner). -->
  <ViewToolbar role="toolbar" :aria-label="t('git.toolbar.region.aria')">
    <RefsPopover
      ref="refsPopoverRef"
      :refs="refsState"
      :ops="opsState"
      :stash="stashState"
      :worktrees="worktreeState"
      :stack="stackState"
      :change-requests="changeRequestState"
      :open-change-request="openChangeRequest"
      @branchFromStash="(entry) => emit('branchFromStash', entry)"
      @saveGlobalStash="emit('saveGlobalStash')"
      @saveEntryToGlobalStash="(entry) => emit('saveEntryToGlobalStash', entry)"
      @switchWorktree="(path) => emit('switchWorktree', path)"
      @createWorktree="(seed) => emit('createWorktree', seed)"
      @openRestackDialog="(branch) => emit('openRestackDialog', branch)"
      @openSetStackParentDialog="(branch) => emit('openSetStackParentDialog', branch)"
    />
    <span
      v-if="stackState.restacking.value"
      class="inline-flex items-center h-control-sm px-1.5 rounded-kira-sm gap-1 text-muted-foreground text-kira-sm whitespace-nowrap"
      role="status"
      aria-live="polite"
    >
      <Spinner icon="sync" label="" />
      {{ t('git.toolbar.restacking.loading') }}
    </span>
    <ToolbarSeparator />
    <RefreshButton
      ref="refreshButtonRef"
      :graph-view="graphView"
      :repo-state="repoState"
      :report-error="reportError"
    />

    <template v-if="hasRemote">
      <ToolbarSeparator />
      <TextTooltip :text="t('common.fetchTarget.tooltip', { name: defaultRemote ?? '' })">
        <Button
          variant="toolbar"
          size="kira"
          :disabled="fetchDisabled"
          data-testid="fetch-button"
          @click="doFetch"
        >
          <CodiconIcon name="cloud-download" :size="13" />
          {{ t('git.toolbar.fetch.label') }}
        </Button>
      </TextTooltip>

      <PullStrategyPicker
        v-if="currentBranch !== undefined"
        ref="pullStrategyPickerRef"
        :ops="opsState"
        :remote="defaultRemote as string"
        :branch="currentBranch"
        :disabled="pushPullDisabled"
      />

      <SplitButton
        v-model:open="isForcePushMenuOpen"
        :disabled="pushPullDisabled"
        :tooltip="t('git.toolbar.push.tooltip', { remote: defaultRemote ?? '' })"
        :chevron-label="t('git.toolbar.pushOptions.aria')"
        chevron-testid="push-overflow-trigger"
        data-testid="push-button"
        @click="doPush"
      >
        <CodiconIcon name="repo-push" :size="13" />
        {{ t('git.toolbar.push.label') }}
        <template #menu>
          <DropdownMenuContent align="end" :aria-label="t('git.toolbar.pushOptions.aria')">
            <MenuItems :items="pushMenuItems" />
          </DropdownMenuContent>
        </template>
      </SplitButton>
    </template>

    <TextTooltip v-if="autoFetchStopped !== null" :text="autoFetchTooltip"
      ><Button
        variant="toolbar"
        size="kira"
        data-testid="autofetch-stopped"
        @click="onAutoFetchMarkerClick"
      >
        <CodiconIcon name="warning" :size="13" class="text-warn" />
        {{ t('git.toolbar.autoFetchStopped.label') }}
      </Button></TextTooltip
    >

    <ToolbarSeparator />
    <TextTooltip :text="t('git.toolbar.stash.tooltip')">
      <Button
        variant="toolbar"
        size="kira"
        :disabled="stashDisabled"
        data-testid="stash-changes-button"
        @click="emit('stashChanges')"
      >
        <CodiconIcon name="inbox" :size="13" />
        {{ t('git.toolbar.stash.label') }}
      </Button>
    </TextTooltip>

    <span class="flex-1" aria-hidden="true"></span>

    <TooltipIconButton
      icon="list-tree"
      :label="t('git.toolbar.collapseBranches.tooltip')"
      :pressed="collapseBranches"
      data-testid="graph-collapse-toggle"
      @click="emit('toggleCollapseBranches')"
    />

    <TooltipIconButton
      icon="search"
      :label="t('common.action.search.label')"
      :pressed="searchOpen"
      data-testid="search-toggle-button"
      @click="emit('toggleSearch')"
    />

    <TooltipIconButton
      :icon="ICON.settings"
      :label="t('git.toolbar.repoSettings.tooltip')"
      data-testid="repo-settings-button"
      @click="emit('openRepoSettings')"
    />

    <div
      v-if="remoteBusy"
      class="inline-flex items-center h-control-sm px-1.5 rounded-kira-sm gap-1 text-muted-foreground text-kira-sm"
      data-testid="remote-progress"
    >
      <Spinner label="" />
      <span class="max-w-65 truncate">{{ progressText }}</span>
      <TooltipIconButton
        :icon="ICON.stop"
        :label="cancellable ? t('common.action.cancel.label') : (cancelDisabledReason ?? '')"
        :disabled="!cancellable"
        disabled-trigger
        data-testid="remote-cancel"
        @click="doCancel"
      />
    </div>

    <UndoButton :ops="opsState" :copy="copy" />
  </ViewToolbar>
</template>
