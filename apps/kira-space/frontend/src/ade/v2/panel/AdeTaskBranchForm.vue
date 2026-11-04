<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref, watch } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import { candidateOf, candidateOptions, newBranchError } from '../branchOptions';
import { useCandidates, useSetTaskBranch, useUpdateTaskRepo } from '../queries';
import type { Branch } from '../wire';
import { useDiscardGuard } from './useDiscardGuard';

// Popover of one task row: pick the row's branch (existing, or a typed new name) or its repo.
const props = defineProps<{
  branch: Branch;
  mode: 'branch' | 'repo';
  /** Repos where the task has a branch slot waiting for a name. */
  draftRepoIds: readonly string[];
}>();
const open = defineModel<boolean>('open', { required: true });

const error = ref('');
watch(open, (o) => {
  if (o) error.value = '';
});

const candidates = useCandidates(
  () => props.branch.codeRepoId,
  () => open.value && props.mode === 'branch',
);
const branches = computed(() =>
  candidateOptions(candidates.data.value?.branches ?? [], () => '', Date.now()),
);
const projects = useProjectOptions(() => props.draftRepoIds);

const guard = useDiscardGuard();
const setBranch = useSetTaskBranch();
const setRepo = useUpdateTaskRepo();

async function submitBranch(name: string, existing: boolean): Promise<void> {
  error.value = '';
  const branchId = props.branch.id;
  const run = async (discardChanges: boolean): Promise<void> => {
    await setBranch.mutateAsync({ branchId, name, existing, discardChanges });
    open.value = false;
  };
  try {
    // A not-created row has no worktree to lose.
    if (props.branch.name === '') await run(false);
    else
      await guard(
        {
          branchId,
          name: props.branch.name,
          title: t('space.ade.taskTab.branchMenu.changeBranchConfirm.title'),
          body: (risk) =>
            t('space.ade.taskTab.branchMenu.leaveConfirm.body', { name: props.branch.name, risk }),
          confirmLabel: t('space.ade.taskTab.branchMenu.changeBranchConfirm.confirm'),
        },
        run,
      );
  } catch (err) {
    error.value = errorText(err);
  }
}

async function submitRepo(codeRepoId: string): Promise<void> {
  error.value = '';
  try {
    await setRepo.mutateAsync({ branchId: props.branch.id, codeRepoId });
    open.value = false;
  } catch (err) {
    error.value = errorText(err);
  }
}
</script>

<template>
  <Popover v-model:open="open">
    <PopoverAnchor as-child><slot /></PopoverAnchor>
    <PopoverContent
      align="end"
      class="w-96 gap-0 overflow-hidden p-0"
      data-testid="ade-branch-form"
    >
      <BranchPicker
        v-if="mode === 'branch'"
        inline
        testid="ade-branch-form"
        :branches="branches"
        :placeholder="t('common.action.choose.label')"
        :loading="candidates.isPending.value"
        :create="{ validate: newBranchError }"
        @select="(o) => submitBranch(candidateOf(o.value).name, true)"
        @create="(name) => submitBranch(name, false)"
      />
      <ProjectPicker
        v-else
        inline
        testid="ade-branch-form-repo"
        :projects="projects.options.value"
        :placeholder="t('common.action.choose.label')"
        :loading="projects.isPending.value"
        :error="projects.error.value"
        @select="(o) => submitRepo(o.value)"
      />
      <div v-if="error" class="m-2">
        <Alert variant="error" data-testid="ade-branch-form-error"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
      </div>
    </PopoverContent>
  </Popover>
</template>
