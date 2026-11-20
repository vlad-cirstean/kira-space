<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';
import { t } from '@theme/i18n';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import BranchPicker from '@workbench/components/pickers/BranchPicker.vue';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref, watch } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import { candidateOf, candidateOptions, newBranchError } from '../branchOptions';
import { useCandidates, useSetTaskBranch, useUpdateTaskRepo } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdeOpsStore } from '../state/adeOps';
import type { Branch } from '../wire';

// Popover of one task row: pick the row's branch (existing, or a typed new name) or its repo.
const props = defineProps<{
  branch: Branch;
  mode: 'branch' | 'repo';
  /** Repos where the task has a branch slot waiting for a name. */
  draftRepoIds: readonly string[];
  align?: 'start' | 'end';
}>();
const open = defineModel<boolean>('open', { required: true });

const surface = useTaskAction();
const error = surface.error;
// The choice a dirty row holds until the user discards its changes.
const held = ref<{ name: string; existing: boolean } | null>(null);
// A not-created row has no worktree to lose.
const dirtyCount = computed(() => (props.branch.name === '' ? 0 : props.branch.dirty.length));
watch(open, (o) => {
  if (o) surface.reset();
  else held.value = null;
});

const candidates = useCandidates(
  () => props.branch.codeRepoId,
  () => open.value && props.mode === 'branch',
);
const branches = computed(() =>
  candidateOptions(candidates.data.value?.branches ?? [], () => '', Date.now()),
);
const projects = useProjectOptions(() => props.draftRepoIds);

const setBranch = useSetTaskBranch();
const ops = useAdeOpsStore();
const ui = useAdeBoardUiStore();
const setRepo = useUpdateTaskRepo();

async function submitBranch(name: string, existing: boolean, discard = false): Promise<void> {
  const branchId = props.branch.id;
  const taskId = props.branch.taskId;
  if (dirtyCount.value > 0 && !discard) {
    held.value = { name, existing };
    return;
  }
  held.value = null;
  await surface.run(async () => {
    open.value = false;
    // The change runs as the branch's op (chip on the row, error toast on failure).
    const ok = await ops.run(
      { id: `setBranch:${branchId}`, kind: 'setBranch', taskId, branchId },
      () => setBranch.mutateAsync({ branchId, name, existing, discardChanges: discard }),
    );
    if (!ok) ui.select(taskId);
  });
}

async function submitRepo(codeRepoId: string): Promise<void> {
  await surface.run(async () => {
    await setRepo.mutateAsync({ branchId: props.branch.id, codeRepoId });
    open.value = false;
  });
}
</script>

<template>
  <Popover v-model:open="open">
    <PopoverAnchor as-child><slot /></PopoverAnchor>
    <PopoverContent :align="align ?? 'end'" flush class="w-80" data-testid="ade-branch-form">
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
      <div v-if="held" class="flex flex-col gap-1 p-1" data-testid="ade-branch-form-dirty">
        <Alert variant="error" data-testid="ade-branch-form-dirty-error"
          ><AlertDescription>{{
            t('space.ade.taskTab.branchForm.dirty.error', {
              name: branch.name,
              count: dirtyCount,
            })
          }}</AlertDescription></Alert
        >
        <Button
          variant="dialog-danger"
          size="kira"
          data-testid="ade-branch-form-discard"
          @click="submitBranch(held.name, held.existing, true)"
          >{{
            t('space.ade.taskTab.branchForm.discardChange.label', { count: dirtyCount })
          }}</Button
        >
      </div>
      <div v-if="error" class="p-1">
        <Alert variant="error" data-testid="ade-branch-form-error"
          ><AlertDescription>{{ error }}</AlertDescription></Alert
        >
      </div>
    </PopoverContent>
  </Popover>
</template>
