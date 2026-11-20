<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import type { BranchRowModel } from '../plan/usePlanModel';
import { useRemoveTaskBranch, useRemoveTaskRepo } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeOpsStore } from '../state/adeOps';
import AdeTaskBranchForm from './AdeTaskBranchForm.vue';

// Row menu of the Task tab: add or change the branch, change or remove the repo (P397).
const props = defineProps<{
  row: BranchRowModel;
  /** Repos where the task has a branch slot waiting for a name. */
  draftRepoIds: readonly string[];
  align?: 'start' | 'end';
}>();

const ui = useAdeBoardUiStore();
const ops = useAdeOpsStore();
const removeBranch = useRemoveTaskBranch();
const removeRepo = useRemoveTaskRepo();

const formOpen = ref(false);
const menuOpen = ref(false);
const mode = ref<'branch' | 'repo'>('branch');
const dirty = computed(() => props.row.branch.dirty.length);

const menuItems = computed<MenuItem[]>(() => [
  ...(props.row.draft
    ? ([
        {
          type: 'item',
          id: 'add',
          testId: 'ade-task-branch-add',
          label: t('space.ade.taskTab.branchMenu.addBranch.label'),
          icon: 'git-branch',
          run: () => openForm('branch'),
        },
        {
          type: 'item',
          id: 'repo-change',
          testId: 'ade-task-repo-change',
          label: t('space.ade.taskTab.branchMenu.changeRepo.label'),
          icon: 'repo',
          run: () => openForm('repo'),
        },
      ] satisfies MenuItem[])
    : ([
        {
          type: 'item',
          id: 'change',
          testId: 'ade-task-branch-change',
          label: t('space.ade.taskTab.branchMenu.changeBranch.label'),
          icon: 'git-branch',
          run: () => openForm('branch'),
        },
        {
          type: 'item',
          id: dirty.value ? 'remove-discard' : 'remove',
          testId: dirty.value ? 'ade-task-branch-remove-discard' : 'ade-task-branch-remove',
          label: dirty.value
            ? t('space.ade.taskTab.branchMenu.discardRemoveBranch.label', { count: dirty.value })
            : t('space.ade.taskTab.branchMenu.removeBranch.label'),
          icon: 'trash',
          danger: true,
          run: () => remove('branch'),
        },
      ] satisfies MenuItem[])),
  { type: 'separator' },
  {
    type: 'item',
    id: dirty.value ? 'repo-remove-discard' : 'repo-remove',
    testId: dirty.value ? 'ade-task-repo-remove-discard' : 'ade-task-repo-remove',
    label: dirty.value
      ? t('space.ade.taskTab.branchMenu.discardRemoveRepo.label', { count: dirty.value })
      : t('space.ade.taskTab.branchMenu.removeRepo.label'),
    icon: 'trash',
    danger: true,
    run: () => remove('repo'),
  },
]);

function openForm(next: 'branch' | 'repo'): void {
  mode.value = next;
  formOpen.value = true;
}

// A dirty row's item is the explicit discard; the server re-reads the changes and refuses a row that turned
// dirty after the board did.
function remove(what: 'branch' | 'repo'): void {
  const branchId = props.row.id;
  const taskId = props.row.branch.taskId;
  // The token of the changes the menu item named; the server refuses a set that changed since.
  const discardToken = dirty.value > 0 ? props.row.branch.dirtyToken : '';
  delete ui.actionError[taskId];
  const mutation = what === 'branch' ? removeBranch : removeRepo;
  ops.run(
    {
      id: `${what === 'branch' ? 'removeBranch' : 'removeRepo'}:${branchId}`,
      kind: what === 'branch' ? 'removeBranch' : 'removeRepo',
      taskId,
      branchId,
    },
    async () => {
      await mutation.mutateAsync({ branchId, discardToken });
      if (ui.selectedBranchId === branchId) ui.select(taskId);
    },
  );
}
</script>

<template>
  <AdeTaskBranchForm
    v-model:open="formOpen"
    :branch="row.branch"
    :mode="mode"
    :draft-repo-ids="draftRepoIds"
    :align="align"
  >
    <DropdownMenu v-model:open="menuOpen">
      <DropdownMenuTrigger as-child>
        <Button
          variant="toolbar"
          size="kira-icon"
          class="shrink-0"
          :aria-label="t('space.ade.taskTab.branchMenu.trigger.aria')"
          :data-pinned="menuOpen || formOpen || undefined"
          data-testid="ade-task-branch-menu"
          :data-branch-id="row.id"
          @click.stop
        >
          <CodiconIcon :name="ICON.edit" :size="13" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        :align="align ?? 'end'"
        data-testid="ade-task-branch-menu-content"
        @close-auto-focus.prevent
      >
        <MenuItems :items="menuItems" />
      </DropdownMenuContent>
    </DropdownMenu>
  </AdeTaskBranchForm>
</template>
