<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import { useOpenHostView } from '../../../hostview/queries';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import AdeForcePushDialog from '../AdeForcePushDialog.vue';
import AdeTip from '../AdeTip.vue';
import AdeBranchRow from '../plan/AdeBranchRow.vue';
import { CHECKS_GLYPH, PR_GLYPH } from '../plan/glyphs';
import type { CardModel } from '../plan/usePlanModel';
import { useAddTaskRepo, useChangeRequests } from '../queries';
import { useAdeAddUiStore } from '../state/adeAddUi';
import { TONE_TEXT_CLASS } from '../tones';
import AdeTaskBranchMenu from './AdeTaskBranchMenu.vue';
import { PR_TONE, prStateLabel } from './prState';

// Main tab branches: the repo and branch adders, then one row per branch.
const props = defineProps<{ card: CardModel }>();
const emit = defineEmits<{ error: [message: string] }>();

const addUi = useAdeAddUiStore();
const addRepo = useAddTaskRepo();
const changeRequests = useChangeRequests();
const openView = useOpenHostView();

const lines = computed(() => {
  const byBranch = changeRequests.data.value?.branches ?? {};
  return props.card.rows.map((row) => ({ row, pr: byBranch[row.id] }));
});
const forcePushTarget = ref<{ branchId: string; branchName: string; repo: string } | null>(null);

const task = computed(() => props.card.task);
const review = computed(() => props.card.review);

const usedRepoIds = computed(() => props.card.rows.map((r) => r.branch.codeRepoId));
const addProjects = useProjectOptions(usedRepoIds);
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
  <div class="flex flex-col gap-0.5">
    <div class="flex items-center gap-2 pb-0.5">
      <span class="text-kira-sm text-subtle">{{ t('common.module.branches.label') }}</span>
      <span class="flex-1" />
      <template v-if="!review">
        <ProjectPicker
          v-if="addProjects.options.value.length > 0"
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
      </template>
      <Button
        variant="dialog"
        size="kira"
        data-testid="ade-add-branch"
        @click="addUi.openAttach(task.id, card.title)"
      >
        {{ t('space.ade.taskTab.addBranch.label') }}
      </Button>
    </div>
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
        testid="ade-task-branch"
        @force-push="forcePushTarget = { branchId: row.id, branchName: row.name, repo: row.repo }"
      >
        <AdeTaskBranchMenu
          v-if="!review"
          :row="row"
          :used-repo-ids="usedRepoIds"
          @error="(m: string) => emit('error', m)"
        />
      </AdeBranchRow>
      <div
        v-if="pr"
        class="flex min-w-0 items-center gap-2 pb-1 text-kira-sm"
        :style="{ paddingLeft: `${row.depth * 16 + 8}px` }"
        data-testid="ade-task-branch-pr"
        :data-branch-id="row.id"
      >
        <AdeTip :text="prStateLabel(pr.state)">
          <span
            class="shrink-0"
            :class="TONE_TEXT_CLASS[PR_TONE[pr.state] ?? 'grey']"
            :data-state="pr.state"
            data-testid="ade-task-branch-pr-state"
            ><CodiconIcon :name="PR_GLYPH[pr.state] ?? 'git-pull-request'" :size="12" /></span
          >
        </AdeTip>
        <a
          :href="pr.url"
          class="min-w-0 truncate text-info"
          @click.prevent="fireAndForget(openView.mutateAsync(pr.url), 'hostView.open')"
          >{{
            pr.title
          }}</a
        >
        <span
          v-if="pr.baseRef"
          class="shrink-0 font-data text-subtle"
          data-testid="ade-task-branch-pr-base"
          >{{
            t('space.ade.mainTab.prBase.label', { base: pr.baseRef })
          }}</span
        >
        <AdeTip v-if="CHECKS_GLYPH[pr.checksState]" :text="t(CHECKS_GLYPH[pr.checksState]!.tip)">
          <span
            class="shrink-0"
            :class="CHECKS_GLYPH[pr.checksState]!.class"
            :data-state="pr.checksState"
            data-testid="ade-task-branch-pr-checks"
            ><CodiconIcon :name="CHECKS_GLYPH[pr.checksState]!.icon" :size="12" /></span
          >
        </AdeTip>
      </div>
    </div>
    <AdeForcePushDialog :target="forcePushTarget" @close="forcePushTarget = null" />
  </div>
</template>
