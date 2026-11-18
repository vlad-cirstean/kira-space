<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { TooltipDisabledTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowMeta } from '@theme/lib/rowVariants';
import { useKeptScroll } from '@workbench/behaviour/viewState';
import { errorText } from '@workbench/bridge/codedError';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed, ref, useTemplateRef } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeParentChip from '../AdeParentChip.vue';
import { orchestratorToggle } from '../board/kind';
import { statusWhy, taskPatch } from '../board/panelFacts';
import AdeTaskChildren from '../plan/AdeTaskChildren.vue';
import type { CardModel } from '../plan/usePlanModel';
import { useUpdateTask } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import AdeTaskUpdates from '../updates/AdeTaskUpdates.vue';
import type { TaskPatch } from '../wire';
import AdeEstimateField from './AdeEstimateField.vue';
import AdeMainBranches from './AdeMainBranches.vue';
import AdeStatusPicker from './AdeStatusPicker.vue';
import AdeWorkflowBlock from './AdeWorkflowBlock.vue';
import { useEstimateWrite } from './useEstimateWrite';

// Main tab: the editable fields, the branches, then the workflow.
const props = defineProps<{ card: CardModel }>();

const approvalsStore = useApprovalsStore();
const update = useUpdateTask();
const boardUi = useAdeBoardUiStore();

const task = computed(() => props.card.task);
const review = computed(() => props.card.review);
const fieldError = ref('');
const est = useEstimateWrite(() => task.value.id);

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
const spanDays = computed(() => props.card.entry.span);

// ---- kind
const toggle = computed(() => orchestratorToggle(props.card));

const bodyEl = useTemplateRef<HTMLElement>('bodyEl');
useKeptScroll(() => `ade:task:${props.card.task.id}:main`, bodyEl);
</script>

<template>
  <div
    ref="bodyEl"
    class="flex min-h-0 flex-1 flex-col gap-3 overflow-auto px-1.5 py-3 text-kira-md"
    data-testid="ade-main-tab"
  >
    <div class="@container flex flex-col gap-2 px-1.5" data-testid="ade-main-fields">
      <Field orientation="responsive">
        <FieldLabel>{{ t('common.field.status.label') }}</FieldLabel>
        <div class="flex min-w-0 items-center gap-1.5">
          <AdeStatusPicker :card="card" />
          <AdeChip
            v-if="card.blocked"
            :label="t('space.common.status.blocked.status')"
            tone="red"
            data-testid="ade-task-blocked"
          />
          <span class="truncate" :class="rowMeta" data-testid="ade-task-status-why">{{ why }}</span>
        </div>
      </Field>
      <Field orientation="responsive">
        <FieldLabel for="ade-est-num">{{ t('space.ade.estimate.field.label') }}</FieldLabel>
        <div class="flex min-w-0 items-center gap-1.5">
          <AdeEstimateField
            :est="task.est"
            :days="spanDays"
            :error="est.error.value"
            @save="est.save"
          />
        </div>
      </Field>
      <Field
        v-if="task.branchRequestId"
        orientation="responsive"
        data-testid="ade-task-branch-request-row"
      >
        <FieldLabel>{{ t('space.ade.taskTab.branchRequest.label') }}</FieldLabel>
        <div class="flex min-w-0 items-center gap-1.5">
          <span>{{ t('space.ade.taskTab.branchRequest.status') }}</span>
          <Button
            variant="toolbar"
            size="kira"
            class="ml-auto"
            data-testid="ade-task-branch-request-open"
            @click="approvalsStore.open(`branch.request:${task.branchRequestId}`)"
            ><CodiconIcon name="git-compare" :size="13" />
            {{ t('space.ade.board.branchTag.review.label') }}</Button
          >
        </div>
      </Field>
      <Field v-if="card.parent" orientation="responsive" data-testid="ade-task-parent-row">
        <FieldLabel>{{ t('space.ade.orchestrator.parent.label') }}</FieldLabel>
        <div class="flex min-w-0 items-center gap-1.5">
          <AdeParentChip
            :parent="card.parent"
            @select="card.parent && boardUi.select(card.parent.id)"
          />
        </div>
      </Field>
      <Field v-if="!review" orientation="responsive">
        <FieldLabel for="ade-task-parked">{{ t('space.ade.board.notMerging.label') }}</FieldLabel>
        <div class="flex min-w-0 items-center gap-1.5">
          <TextTooltip
            :text="card.orchestrator ? t('space.ade.orchestrator.parkRefused.tooltip') : ''"
          >
            <TooltipDisabledTrigger :disabled="card.orchestrator">
              <Switch
                id="ade-task-parked"
                :model-value="card.parked"
                :disabled="card.orchestrator"
                data-testid="ade-task-parked"
                @update:model-value="(v: boolean) => write({ kind: v ? 'parked' : 'task' })"
              />
            </TooltipDisabledTrigger>
          </TextTooltip>
          <span class="truncate" :class="rowMeta">{{
            t('space.ade.taskTab.notMerging.hint')
          }}</span>
        </div>
      </Field>
      <Field v-if="!review" orientation="responsive" data-testid="ade-task-orchestrator-row">
        <FieldLabel for="ade-task-orchestrator">{{
          t('space.ade.orchestrator.badge.label')
        }}</FieldLabel>
        <div class="flex min-w-0 items-center gap-1.5">
          <TextTooltip :text="toggle.why">
            <TooltipDisabledTrigger :disabled="toggle.disabled">
              <Switch
                id="ade-task-orchestrator"
                :model-value="card.orchestrator"
                :disabled="toggle.disabled"
                data-testid="ade-task-orchestrator"
                @update:model-value="(v: boolean) => write({ kind: v ? 'orchestrator' : 'task' })"
              />
            </TooltipDisabledTrigger>
          </TextTooltip>
          <span class="truncate" :class="rowMeta">{{
            t('space.ade.orchestrator.switch.hint')
          }}</span>
        </div>
      </Field>
      <Alert v-if="fieldError" variant="error" data-testid="ade-task-error"
        ><AlertDescription>{{ fieldError }}</AlertDescription></Alert
      >
    </div>

    <div v-if="card.orchestrator" class="flex flex-col gap-1 px-1.5">
      <span class="text-kira-sm text-subtle">{{ t('space.ade.orchestrator.children.label') }}</span>
      <AdeTaskChildren :card="card" testid="ade-main-children" />
      <span
        v-if="!card.children.length && !card.archivedChildren.length"
        class="text-muted-foreground"
        data-testid="ade-main-children-empty"
        >{{
          t('space.ade.orchestrator.children.empty')
        }}</span
      >
    </div>

    <AdeTaskUpdates :task-id="task.id" />

    <AdeMainBranches :card="card" @error="(m: string) => (fieldError = m)" />

    <AdeWorkflowBlock v-if="!review && !card.parked" :card="card" />
  </div>
</template>
