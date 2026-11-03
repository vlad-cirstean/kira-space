<script setup lang="ts">
import KeyValueList from '@theme/components/KeyValueList.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Label } from '@theme/components/ui/label';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import { statusWhy, taskPatch } from '../board/panelFacts';
import type { CardModel } from '../plan/usePlanModel';
import { useUpdateTask } from '../queries';
import { useAdeBranchRequestStore } from '../state/adeBranchRequest';
import type { TaskPatch } from '../wire';
import AdeEstimateField from './AdeEstimateField.vue';
import AdeMainBranches from './AdeMainBranches.vue';
import AdeStatusPicker from './AdeStatusPicker.vue';
import AdeWorkflowBlock from './AdeWorkflowBlock.vue';

// Main tab: the editable fields, the branches, then the workflow.
const props = defineProps<{ card: CardModel }>();

const requestStore = useAdeBranchRequestStore();
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
    class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-3 pb-3 pt-3 text-kira-md"
    data-testid="ade-main-tab"
  >
    <KeyValueList class="items-center">
      <dt>{{ t('common.field.status.label') }}</dt>
      <dd class="flex min-h-row items-center gap-2">
        <AdeStatusPicker :card="card" />
        <AdeChip
          v-if="card.blocked"
          :label="t('space.common.status.blocked.status')"
          tone="red"
          data-testid="ade-task-blocked"
        />
        <span class="text-kira-sm text-subtle" data-testid="ade-task-status-why">{{ why }}</span>
      </dd>
      <dt>{{ t('space.ade.estimate.field.label') }}</dt>
      <dd
        ><AdeEstimateField :est="task.est" :days="spanDays" :error="estError" @save="saveEst" /></dd
      >
      <template v-if="task.branchRequestId">
        <dt>{{ t('space.ade.taskTab.branchRequest.label') }}</dt>
        <dd class="flex min-h-row items-center gap-2" data-testid="ade-task-branch-request-row">
          <span class="text-kira-md">{{ t('space.ade.taskTab.branchRequest.status') }}</span>
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="ade-task-branch-request-open"
            @click="requestStore.open({ id: task.branchRequestId })"
            >{{
              t('space.ade.board.branchTag.review.label')
            }}</Button
          >
        </dd>
      </template>
      <template v-if="!review">
        <dt />
        <dd class="flex min-h-row items-center gap-2">
          <Switch
            id="ade-task-parked"
            :model-value="card.parked"
            data-testid="ade-task-parked"
            @update:model-value="(v: boolean) => write({ kind: v ? 'parked' : 'task' })"
          />
          <Label for="ade-task-parked" class="text-kira-md">{{
            t('space.ade.board.notMerging.label')
          }}</Label>
          <span class="text-kira-sm text-subtle">{{ t('space.ade.taskTab.notMerging.hint') }}</span>
        </dd>
      </template>
    </KeyValueList>
    <Alert v-if="fieldError" variant="error" data-testid="ade-task-error"
      ><AlertDescription>{{ fieldError }}</AlertDescription></Alert
    >

    <AdeMainBranches :card="card" @error="(m: string) => (fieldError = m)" />

    <AdeWorkflowBlock v-if="!review && !card.parked" :card="card" />
  </div>
</template>
