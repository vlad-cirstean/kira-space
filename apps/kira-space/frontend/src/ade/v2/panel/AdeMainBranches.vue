<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { errorText } from '@workbench/bridge/codedError';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import AdeForcePushDialog from '../AdeForcePushDialog.vue';
import type { ForcePushTarget } from '../board/panelFacts';
import AdeBranchRow from '../plan/AdeBranchRow.vue';
import type { CardModel } from '../plan/usePlanModel';
import PrGlyph from '../pr/PrGlyph.vue';
import { useAddTaskRepo, useChangeRequests } from '../queries';
import { useAdeAddUiStore } from '../state/adeAddUi';
import AdeUpdatesDot from '../updates/AdeUpdatesDot.vue';
import { useTaskUpdates } from '../updates/useTaskUpdates';
import AdeRow from './AdeRow.vue';
import AdeTaskBranchMenu from './AdeTaskBranchMenu.vue';
import { useOpenBranchPr } from './useOpenBranchPr';

// Main tab branches: the repo and branch adders, then one row per branch.
const props = defineProps<{ card: CardModel }>();
const emit = defineEmits<{ error: [message: string] }>();

const addUi = useAdeAddUiStore();
const addRepo = useAddTaskRepo();
const changeRequests = useChangeRequests();

const lines = computed(() => {
  const byBranch = changeRequests.data.value?.branches ?? {};
  const groups = new Map<string, typeof props.card.rows>();
  for (const row of props.card.rows) {
    const group = groups.get(row.branch.codeRepoId) ?? [];
    group.push(row);
    groups.set(row.branch.codeRepoId, group);
  }
  return [...groups.values()].flat().map((row) => ({ row, pr: byBranch[row.id] }));
});
const { byBranch, describe } = useTaskUpdates();
const openBranchPr = useOpenBranchPr();
const forcePushTarget = ref<ForcePushTarget | null>(null);

const task = computed(() => props.card.task);
const review = computed(() => props.card.review);

const draftRepoIds = computed(() =>
  props.card.rows.filter((r) => r.branch.name === '').map((r) => r.branch.codeRepoId),
);
const addProjects = useProjectOptions(draftRepoIds);
const addRepoValue = ref<string | null>(null);
async function onAddRepo(id: string): Promise<void> {
  addRepoValue.value = null;
  emit('error', '');
  try {
    await addRepo.mutateAsync({ taskId: task.value.id, codeRepoId: id, base: null });
  } catch (err) {
    emit('error', errorText(err));
  }
}
</script>

<template>
  <div class="flex flex-col">
    <SectionHeading :label="t('common.module.branches.label')">
      <ProjectPicker
        v-if="!review && addProjects.options.value.length > 0"
        v-model="addRepoValue"
        size="kira"
        testid="ade-add-repo"
        :projects="addProjects.options.value"
        :placeholder="t('space.ade.taskTab.addRepo.option')"
        :aria-label="t('space.ade.taskTab.addRepo.aria')"
        :title="t('space.ade.taskTab.addRepo.tooltip')"
        class="max-w-50"
        @select="(o) => onAddRepo(o.value)"
      />
      <Button
        variant="toolbar"
        size="kira"
        data-testid="ade-add-branch"
        @click="addUi.openAttach(task.id, card.title)"
        ><CodiconIcon name="add" :size="13" />{{ t('space.ade.taskTab.addBranch.label') }}
      </Button>
    </SectionHeading>
    <div
      v-for="{ row, pr } in lines"
      :key="row.id"
      class="flex flex-col"
      data-testid="ade-task-branch-item"
      :data-branch-id="row.id"
    >
      <AdeBranchRow
        :row="row"
        :color="card.color"
        layout="panel"
        testid="ade-task-branch"
        @force-push="
          forcePushTarget = {
            branchId: row.id,
            branchName: row.name,
            repo: row.repo,
            taskId: row.branch.taskId,
          }
        "
      >
        <AdeUpdatesDot
          v-if="byBranch.has(row.id)"
          :tip="(byBranch.get(row.id) ?? []).map(describe).join('; ')"
          :testid="`ade-branch-updated-${row.id}`"
        />
        <AdeTaskBranchMenu
          v-if="!review"
          :row="row"
          :draft-repo-ids="draftRepoIds"
          @error="(m: string) => emit('error', m)"
        />
      </AdeBranchRow>
      <AdeRow
        v-if="pr"
        :depth="row.depth + 1"
        data-testid="ade-task-branch-pr"
        :data-branch-id="row.id"
      >
        <template #lead>
          <PrGlyph variant="tooltip" :status="pr" testid="ade-task-branch-pr-state" />
        </template>
        <a
          :href="pr.url"
          class="min-w-0 truncate text-info"
          @click.prevent="openBranchPr.open(card.task.id, row.id, pr.url)"
          >{{
            pr.title
          }}</a
        >
        <span
          v-if="pr.baseRef"
          class="shrink-0 font-data"
          :class="rowMeta"
          data-testid="ade-task-branch-pr-base"
          >{{
            t('space.ade.mainTab.prBase.label', { base: pr.baseRef })
          }}</span
        >
      </AdeRow>
    </div>
    <AdeForcePushDialog :target="forcePushTarget" @close="forcePushTarget = null" />
  </div>
</template>
