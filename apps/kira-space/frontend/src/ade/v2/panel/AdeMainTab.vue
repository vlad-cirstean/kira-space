<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { errorText } from '@workbench/bridge/codedError';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import { statusWhy, taskPatch } from '../board/panelFacts';
import type { CardModel } from '../plan/usePlanModel';
import { useUpdateTask } from '../queries';
import AdeTaskUpdates from '../updates/AdeTaskUpdates.vue';
import type { TaskPatch } from '../wire';
import AdeEstimateField from './AdeEstimateField.vue';
import AdeMainBranches from './AdeMainBranches.vue';
import AdeRow from './AdeRow.vue';
import AdeRowDetail from './AdeRowDetail.vue';
import AdeStatusPicker from './AdeStatusPicker.vue';
import AdeWorkflowBlock from './AdeWorkflowBlock.vue';

// Main tab: the editable fields, the branches, then the workflow.
const props = defineProps<{ card: CardModel }>();

const approvalsStore = useApprovalsStore();
const update = useUpdateTask();

const task = computed(() => props.card.task);
const review = computed(() => props.card.review);
const fieldError = ref('');
const estError = ref<string | null>(null);

async function write(patch: Partial<TaskPatch>): Promise<boolean> {
  fieldError.value = '';
  try {
    await update.mutateAsync({ taskId: task.value.id, patch: taskPatch(patch) });
    return true;
  } catch (err) {
    fieldError.value = errorText(err);
    return false;
  }
}

// ---- status
const why = computed(() => statusWhy(task.value, props.card.blocked, props.card.progress));

// ---- estimate
async function saveEst(value: string): Promise<void> {
  estError.value = null;
  try {
    await update.mutateAsync({ taskId: task.value.id, patch: taskPatch({ est: value }) });
  } catch (err) {
    estError.value = errorText(err);
  }
}
const spanDays = computed(() => props.card.entry.span);
</script>

<template>
  <div
    class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-1.5 py-3 text-kira-md"
    data-testid="ade-main-tab"
  >
    <dl class="m-0 flex flex-col">
      <AdeRow term :label="t('common.field.status.label')">
        <AdeStatusPicker :card="card" />
        <AdeChip
          v-if="card.blocked"
          :label="t('space.common.status.blocked.status')"
          tone="red"
          data-testid="ade-task-blocked"
        />
        <span class="truncate" :class="rowMeta" data-testid="ade-task-status-why">{{ why }}</span>
      </AdeRow>
      <AdeRow term :label="t('space.ade.estimate.field.label')" label-for="ade-est-num">
        <AdeEstimateField :est="task.est" :days="spanDays" :error="estError" @save="saveEst" />
      </AdeRow>
      <AdeRow
        v-if="task.branchRequestId"
        term
        :label="t('space.ade.taskTab.branchRequest.label')"
        data-testid="ade-task-branch-request-row"
      >
        <span>{{ t('space.ade.taskTab.branchRequest.status') }}</span>
        <template #actions>
          <Button
            variant="dialog"
            size="kira"
            data-testid="ade-task-branch-request-open"
            @click="approvalsStore.open(`branch.request:${task.branchRequestId}`)"
            >{{
              t('space.ade.board.branchTag.review.label')
            }}</Button
          >
        </template>
      </AdeRow>
      <AdeRow v-if="!review" term>
        <template #label>
          <Label for="ade-task-parked" class="text-kira-sm font-normal text-muted-foreground">{{
            t('space.ade.board.notMerging.label')
          }}</Label>
        </template>
        <Switch
          id="ade-task-parked"
          :model-value="card.parked"
          data-testid="ade-task-parked"
          @update:model-value="(v: boolean) => write({ kind: v ? 'parked' : 'task' })"
        />
        <span class="truncate" :class="rowMeta">{{ t('space.ade.taskTab.notMerging.hint') }}</span>
      </AdeRow>
    </dl>
    <AdeRowDetail v-if="fieldError">
      <Alert variant="error" data-testid="ade-task-error"
        ><AlertDescription>{{ fieldError }}</AlertDescription></Alert
      >
    </AdeRowDetail>

    <AdeTaskUpdates :task-id="task.id" />

    <AdeMainBranches :card="card" @error="(m: string) => (fieldError = m)" />

    <AdeWorkflowBlock v-if="!review && !card.parked" :card="card" />
  </div>
</template>
