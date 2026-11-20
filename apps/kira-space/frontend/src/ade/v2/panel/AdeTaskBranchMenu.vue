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
import { errorText } from '@workbench/bridge/codedError';
import MenuItems from '@workbench/components/MenuItems.vue';
import type { MenuItem } from '@workbench/state/contextMenu';
import { computed, ref } from 'vue';
import type { BranchRowModel } from '../plan/usePlanModel';
import { useRemoveTaskBranch, useRemoveTaskRepo } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeOpsStore } from '../state/adeOps';
import AdeTaskBranchForm from './AdeTaskBranchForm.vue';
import { useDiscardGuard } from './useDiscardGuard';

// Row menu of the Task tab: add or change the branch, change or remove the repo (P397).
const props = defineProps<{
  row: BranchRowModel;
  /** Repos where the task has a branch slot waiting for a name. */
  draftRepoIds: readonly string[];
  align?: 'start' | 'end';
}>();
const emit = defineEmits<{ error: [message: string] }>();

const ui = useAdeBoardUiStore();
const guard = useDiscardGuard();
const ops = useAdeOpsStore();
const removeBranch = useRemoveTaskBranch();
const removeRepo = useRemoveTaskRepo();

const formOpen = ref(false);
const menuOpen = ref(false);
const mode = ref<'branch' | 'repo'>('branch');
const label = computed(() => props.row.name || props.row.repo);

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
          id: 'remove',
          testId: 'ade-task-branch-remove',
          label: t('space.ade.taskTab.branchMenu.removeBranch.label'),
          icon: 'trash',
          danger: true,
          run: () => remove('branch'),
        },
      ] satisfies MenuItem[])),
  { type: 'separator' },
  {
    type: 'item',
    id: 'repo-remove',
    testId: 'ade-task-repo-remove',
    label: t('space.ade.taskTab.branchMenu.removeRepo.label'),
    icon: 'trash',
    danger: true,
    run: () => remove('repo'),
  },
]);

function openForm(next: 'branch' | 'repo'): void {
  mode.value = next;
  formOpen.value = true;
}

async function remove(what: 'branch' | 'repo'): Promise<void> {
  emit('error', '');
  const branchId = props.row.id;
  const taskId = props.row.branch.taskId;
  try {
    const { proceed, discard } = await guard({
      branchId,
      name: label.value,
      title:
        what === 'branch'
          ? t('space.ade.taskTab.branchMenu.removeBranchConfirm.title')
          : t('space.ade.taskTab.branchMenu.removeRepoConfirm.title'),
      body: (risk) =>
        t('space.ade.taskTab.branchMenu.leaveConfirm.body', { name: label.value, risk }),
      confirmLabel: t('common.action.remove.label'),
    });
    if (!proceed) return;
    const mutation = what === 'branch' ? removeBranch : removeRepo;
    ops.run(
      {
        id: `${what === 'branch' ? 'removeBranch' : 'removeRepo'}:${branchId}`,
        kind: what === 'branch' ? 'removeBranch' : 'removeRepo',
        taskId,
        branchId,
      },
      async () => {
        await mutation.mutateAsync({ branchId, discardChanges: discard });
        if (ui.selectedBranchId === branchId) ui.select(taskId);
      },
    );
  } catch (err) {
    emit('error', errorText(err));
  }
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
