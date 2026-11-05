<script setup lang="ts">
import type { RefRow } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import InlineEditInput from '@workbench/components/InlineEditInput.vue';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { computed, onBeforeUnmount, ref } from 'vue';
import { changeRequestBadgeClass } from '../lib/badgeClass.ts';
import { changeRequestStateLabel } from '../lib/changeRequestStateLabel.ts';
import type { PickerList } from '../lib/pickerModel.ts';
import {
  formatTrack,
  localNameForRemoteBranch,
  remoteCheckoutLabel,
  remoteCheckoutTarget,
} from '../lib/refListModel.ts';
import { buildRefMenu, remoteNamesFrom } from '../lib/rowMenuModel.ts';
import { childOf, parentOf } from '../lib/stackListModel.ts';
import { useSectionMenu } from '../lib/useSectionMenu.ts';
import type { ChangeRequestState } from '../state/changeRequests.ts';
import type { OpsState } from '../state/ops.ts';
import type { RefsState } from '../state/refs.ts';
import type { StackState } from '../state/stack.ts';
import type { WorktreeCreateSeed } from '../state/worktrees.ts';
import RowActionsButton from './RowActionsButton.vue';

// The picker's Branches tab: local and remote branch rows with the ref-scoped menu (W14), inline
// rename, the force-delete prompt and checkout. The picker owns the popover, the filter, the caps
// and the roving focus; this tab asks it to close or to open a right-click menu.
const props = defineProps<{
  refs: RefsState;
  ops: OpsState;
  stack: StackState;
  changeRequests?: ChangeRequestState;
  openChangeRequest: (number: number) => void;
  localSection: PickerList<RefRow>;
  remoteSection: PickerList<RefRow>;
  activeRowId: string | undefined;
  /** Closes the panel after focus moves to the trigger, ahead of a checkout or review. */
  closeForCheckout: () => void;
  /** Right-click: selects the row, then opens the menu at the pointer (B1). */
  openContextMenu: (event: MouseEvent, items: MenuItem[], rowId: string) => void;
}>();
const emit = defineEmits<{
  createWorktree: [seed: WorktreeCreateSeed];
  openRestackDialog: [branch: string];
  openSetStackParentDialog: [branch: string];
}>();

const rowMenu = useSectionMenu();
const contextMenuStore = useContextMenuStore();
onBeforeUnmount(() => rowMenu.close());
const renaming = ref<string | undefined>(undefined);
const forceDeleteCandidate = ref<string | undefined>(undefined);

const knownRemotes = computed(() =>
  remoteNamesFrom(props.refs.remoteBranches.value.map((row) => row.shortName)),
);

/** The one PR record known for a branch's own short name, or `undefined`. */
function changeRequestFor(
  shortName: string,
): { number: number; url: string; title: string; state: string } | undefined {
  return props.changeRequests?.byBranch.value.get(shortName);
}

function changeRequestTooltip(shortName: string): string {
  const pr = changeRequestFor(shortName);
  if (!pr) return '';
  return `${pr.title}\n${changeRequestStateLabel(pr.state)}`;
}

async function checkoutBranch(row: RefRow): Promise<void> {
  props.closeForCheckout();
  await props.ops.runCheckout(row.shortName, 'switch');
}

async function checkoutRemote(row: RefRow): Promise<void> {
  props.closeForCheckout();
  await props.ops.runCheckout(remoteCheckoutTarget(row, props.refs.branches.value), 'switch');
}

function menuFor(row: RefRow) {
  const stackResult = { stacks: props.stack.stacks.value, orphans: props.stack.orphans.value };
  return buildRefMenu({
    kind: row.kind,
    shortName: row.shortName,
    isHead: row.isHead,
    knownRemotes: row.kind === 'tag' ? knownRemotes.value : [],
    inProgress: props.ops.statusSummary.value?.inProgress ?? null,
    stack:
      row.kind === 'branch'
        ? {
            hasParent: parentOf(stackResult, row.shortName) !== undefined,
            hasChild: childOf(stackResult, row.shortName) !== undefined,
          }
        : undefined,
  });
}

function onRowContextMenu(event: MouseEvent, row: RefRow, rowId: string): void {
  props.openContextMenu(
    event,
    rowMenu.itemsFor(menuFor(row), (id) => onRefMenuSelect(row, id)),
    rowId,
  );
}

async function onRefMenuSelect(row: RefRow, id: string): Promise<void> {
  if (id === 'checkoutRef') {
    await (row.kind === 'remoteBranch' ? checkoutRemote(row) : checkoutBranch(row));
    return;
  }
  if (id === 'renameRef') {
    contextMenuStore.afterClose(() => {
      renaming.value = row.shortName;
    });
    return;
  }
  if (id === 'reviewBranch') {
    props.closeForCheckout();
    await props.ops.openReview(row.shortName);
    return;
  }
  if (id === 'deleteRef') {
    const result = await props.ops.branchDelete(row.shortName, false);
    if (!result.ok && result.error?.kind === 'NotFullyMerged') {
      forceDeleteCandidate.value = row.shortName;
    }
    return;
  }
  if (id === 'createWorktreeHere') {
    // A remote-tracking branch's local name may not exist yet, so that arm creates it as a new branch.
    emit(
      'createWorktree',
      row.kind === 'remoteBranch'
        ? {
            mode: 'newBranch',
            branch: localNameForRemoteBranch(row.shortName),
            startPoint: row.shortName,
          }
        : { mode: 'existingBranch', branch: row.shortName },
    );
    return;
  }
  // The stack items: set-parent and restack open dialogs the picker has no place to render;
  // remove is a direct write; the two navigation items check out the neighbour.
  const stackResult = { stacks: props.stack.stacks.value, orphans: props.stack.orphans.value };
  if (id === 'stackSetParent') {
    emit('openSetStackParentDialog', row.shortName);
    return;
  }
  if (id === 'stackRemove') {
    await props.ops.runStackSet(row.shortName, undefined);
    return;
  }
  if (id === 'stackRestack') {
    emit('openRestackDialog', row.shortName);
    return;
  }
  const target =
    id === 'stackGoToParent'
      ? parentOf(stackResult, row.shortName)
      : id === 'stackGoToChild'
        ? childOf(stackResult, row.shortName)
        : undefined;
  if (target !== undefined) {
    props.closeForCheckout();
    await props.ops.runCheckout(target, 'switch');
  }
}

async function confirmForceDelete(): Promise<void> {
  const name = forceDeleteCandidate.value;
  forceDeleteCandidate.value = undefined;
  if (name !== undefined) await props.ops.branchDelete(name, true);
}

async function submitRename(from: string, to: string): Promise<void> {
  renaming.value = undefined;
  await props.ops.branchRename(from, to);
}
</script>

<template>
  <section :aria-label="t('common.module.branches.label')">
    <SectionHeading :label="t('common.module.branches.label')" />
    <div
      data-testid="branch-row"
      v-for="row in localSection.visible"
      :key="row.refname"
      class="flex items-center gap-0.5 px-1"
      :data-row-id="`branch:${row.refname}`"
      :tabindex="activeRowId === `branch:${row.refname}` ? 0 : -1"
    >
      <InlineEditInput
        v-if="renaming === row.shortName"
        data-testid="branch-rename-input"
        :aria-label="t('git.dialog.rename.title')"
        :name="row.shortName"
        @commit="(to) => submitRename(row.shortName, to)"
        @cancel="renaming = undefined"
      />
      <template v-else>
        <button
          data-testid="branch-row-main"
          type="button"
          :class="cn(rowVariants(), 'flex-1 min-w-0 text-left')"
          @click="checkoutBranch(row)"
        >
          <span
            class="w-2.5 text-focus"
            :role="row.isHead ? 'img' : undefined"
            :aria-label="row.isHead ? t('git.common.currentBranch.tooltip') : undefined"
            :aria-hidden="!row.isHead"
            >{{
              row.isHead ? '●' : ''
            }}</span
          >
          <span class="truncate">{{ row.shortName }}</span>
          <Badge
            v-if="row.checkedOutIn"
            :data-kira-tip="t('git.picker.checkedOutIn.tooltip', { path: row.checkedOutIn })"
            >{{
              t('common.worktree.label')
            }}</Badge
          >
          <span v-if="formatTrack(row.track)" :class="rowMeta">{{ formatTrack(row.track) }}</span>
        </button>
        <button
          v-if="changeRequestFor(row.shortName)"
          type="button"
          :class="changeRequestBadgeClass(changeRequestFor(row.shortName)!.state)"
          :data-kira-tip="changeRequestTooltip(row.shortName)"
          @click.stop="openChangeRequest(changeRequestFor(row.shortName)!.number)"
          >#{{ changeRequestFor(row.shortName)!.number }}</button
        >
        <RowActionsButton
          @click="rowMenu.openFromButton($event, menuFor(row), (id) => onRefMenuSelect(row, id))"
          @contextmenu.prevent="onRowContextMenu($event, row, `branch:${row.refname}`)"
        />
      </template>
    </div>
    <Alert v-if="forceDeleteCandidate" variant="warn" strip>
      <AlertDescription>{{
        t('git.common.forceDelete.body', { name: forceDeleteCandidate })
      }}</AlertDescription>
      <div class="flex items-center gap-1">
        <Button variant="dialog-danger" size="kira" @click="confirmForceDelete">{{
          t('git.common.forceDelete.confirm')
        }}</Button>
        <Button variant="toolbar" size="kira" @click="forceDeleteCandidate = undefined">{{
          t('common.action.cancel.label')
        }}</Button>
      </div>
    </Alert>
    <slot name="local-more" />
    <InlineState v-if="localSection.visible.length === 0" kind="empty">{{
      t('git.picker.branches.empty')
    }}</InlineState>
  </section>

  <section :aria-label="t('git.picker.remoteBranches.label')">
    <SectionHeading :label="t('git.picker.remoteBranches.label')" />
    <div
      data-testid="branch-row"
      v-for="row in remoteSection.visible"
      :key="row.refname"
      class="flex items-center gap-0.5 px-1"
      :data-row-id="`remote:${row.refname}`"
      :tabindex="activeRowId === `remote:${row.refname}` ? 0 : -1"
    >
      <button
        data-testid="branch-row-main"
        type="button"
        :class="cn(rowVariants(), 'flex-1 min-w-0 text-left')"
        @click="checkoutRemote(row)"
      >
        <CodiconIcon name="cloud" :size="13" />
        <span class="truncate">{{ row.shortName }}</span>
        <span :class="rowMeta">{{ remoteCheckoutLabel(row, refs.branches.value) }}</span>
      </button>
      <RowActionsButton
        @click="rowMenu.openFromButton($event, menuFor(row), (id) => onRefMenuSelect(row, id))"
        @contextmenu.prevent="onRowContextMenu($event, row, `remote:${row.refname}`)"
      />
    </div>
    <slot name="remote-more" />
    <InlineState v-if="remoteSection.visible.length === 0" kind="empty">
      {{ t('git.picker.remoteBranches.empty') }}
    </InlineState>
  </section>
</template>
