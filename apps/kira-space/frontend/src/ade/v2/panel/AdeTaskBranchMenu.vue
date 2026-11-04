<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import type { BranchRowModel } from '../plan/usePlanModel';
import { useRemoveTaskBranch, useRemoveTaskRepo } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeTaskBranchForm from './AdeTaskBranchForm.vue';
import { useDiscardGuard } from './useDiscardGuard';

// Row menu of the Task tab: add or change the branch, change or remove the repo (P397).
const props = defineProps<{
  row: BranchRowModel;
  /** Repos where the task has a branch slot waiting for a name. */
  draftRepoIds: readonly string[];
}>();
const emit = defineEmits<{ error: [message: string] }>();

const ui = useAdeBoardUiStore();
const guard = useDiscardGuard();
const removeBranch = useRemoveTaskBranch();
const removeRepo = useRemoveTaskRepo();

const formOpen = ref(false);
const mode = ref<'branch' | 'repo'>('branch');
const label = computed(() => props.row.name || props.row.repo);

function openForm(next: 'branch' | 'repo'): void {
  mode.value = next;
  formOpen.value = true;
}

async function remove(what: 'branch' | 'repo'): Promise<void> {
  emit('error', '');
  const branchId = props.row.id;
  try {
    await guard(
      {
        branchId,
        name: label.value,
        title:
          what === 'branch'
            ? t('space.ade.taskTab.branchMenu.removeBranchConfirm.title')
            : t('space.ade.taskTab.branchMenu.removeRepoConfirm.title'),
        body: (risk) =>
          t('space.ade.taskTab.branchMenu.leaveConfirm.body', { name: label.value, risk }),
        confirmLabel: t('common.action.remove.label'),
      },
      async (discardChanges) => {
        await (what === 'branch' ? removeBranch : removeRepo).mutateAsync({
          branchId,
          discardChanges,
        });
        if (ui.selectedBranchId === branchId) ui.select(props.row.branch.taskId);
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
  >
    <DropdownMenu>
      <DropdownMenuTrigger as-child>
        <Button
          variant="toolbar"
          size="kira-icon"
          class="shrink-0"
          :aria-label="t('space.ade.taskTab.branchMenu.trigger.aria')"
          data-testid="ade-task-branch-menu"
          :data-branch-id="row.id"
          @click.stop
        >
          <CodiconIcon :name="ICON.edit" :size="13" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        data-testid="ade-task-branch-menu-content"
        @close-auto-focus.prevent
      >
        <template v-if="row.draft">
          <DropdownMenuItem data-testid="ade-task-branch-add" @select="openForm('branch')">{{
            t('space.ade.taskTab.branchMenu.addBranch.label')
          }}</DropdownMenuItem>
          <DropdownMenuItem data-testid="ade-task-repo-change" @select="openForm('repo')">{{
            t('space.ade.taskTab.branchMenu.changeRepo.label')
          }}</DropdownMenuItem>
        </template>
        <template v-else>
          <DropdownMenuItem data-testid="ade-task-branch-change" @select="openForm('branch')">{{
            t('space.ade.taskTab.branchMenu.changeBranch.label')
          }}</DropdownMenuItem>
          <DropdownMenuItem data-testid="ade-task-branch-remove" @select="remove('branch')">{{
            t('space.ade.taskTab.branchMenu.removeBranch.label')
          }}</DropdownMenuItem>
        </template>
        <DropdownMenuSeparator />
        <DropdownMenuItem data-testid="ade-task-repo-remove" @select="remove('repo')">{{
          t('space.ade.taskTab.branchMenu.removeRepo.label')
        }}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </AdeTaskBranchForm>
</template>
