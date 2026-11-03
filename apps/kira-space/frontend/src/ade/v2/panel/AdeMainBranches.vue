<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ProjectPicker from '@workbench/components/pickers/ProjectPicker.vue';
import { computed, ref } from 'vue';
import { useProjectOptions } from '../../../repo/state/projectOptions';
import AdeChip from '../AdeChip.vue';
import AdeRepoTag from '../AdeRepoTag.vue';
import AdeTip from '../AdeTip.vue';
import { tagLabel } from '../board/actions';
import { integrationChips } from '../board/labels';
import { type CardModel, usePlanModel } from '../plan/usePlanModel';
import { useAddTaskRepo } from '../queries';
import { useAdeAddUiStore } from '../state/adeAddUi';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeTaskBranchMenu from './AdeTaskBranchMenu.vue';

// Main tab branches: the repo and branch adders, then one row per branch.
const props = defineProps<{ card: CardModel }>();
const emit = defineEmits<{ error: [message: string] }>();
const { liveNow } = usePlanModel();

const ui = useAdeBoardUiStore();
const addUi = useAdeAddUiStore();
const addRepo = useAddTaskRepo();

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

function branchLabel(row: CardModel['rows'][number]): string {
  return row.draft ? row.context : row.name;
}

const chipClass = (t: 'muted' | 'stale' | 'unknown'): string =>
  t === 'stale' ? 'text-tone-amber' : 'text-muted-foreground';
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
    <div v-for="row in card.rows" :key="row.id" class="flex items-center gap-1 bg-elevated">
      <Button
        size="kira"
        variant="toolbar"
        class="min-h-row min-w-0 flex-1 justify-start gap-2 bg-elevated px-1.5 text-left text-fg"
        data-testid="ade-task-branch"
        :data-branch-id="row.id"
        @click="ui.selectBranch(task.id, row.id)"
      >
        <AdeChip :label="tagLabel(row.tag, liveNow.getTime())" :tone="row.tag.tone" wide />
        <AdeRepoTag :code-repo-id="row.branch.codeRepoId" :label="row.repo" />
        <AdeTip v-for="c in integrationChips(row.branch)" :key="c.label" :text="c.tip">
          <span class="shrink-0 text-kira-sm font-medium" :class="chipClass(c.tone)">{{
            c.label
          }}</span>
        </AdeTip>
        <span
          class="min-w-0 flex-1 truncate font-data text-kira-md"
          :class="row.draft ? 'text-muted-foreground' : ''"
          >{{
            branchLabel(row)
          }}</span
        >
      </Button>
      <AdeTaskBranchMenu
        v-if="!review"
        :row="row"
        :used-repo-ids="usedRepoIds"
        @error="(m: string) => emit('error', m)"
      />
    </div>
  </div>
</template>
