<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { formatRelative } from '@workbench/behaviour/format';
import { computed, ref } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeOriginSession from '../details/AdeOriginSession.vue';
import { useHandovers, useUpdateHandover } from '../queries';
import { useAdeStageDialogStore } from '../state/adeStageDialog';
import type { Handover } from '../wire';

// What each stage left for the next: the open draft first, then one row per stage move, newest first.
// A row's body is editable unless the task is archived.
const props = defineProps<{ taskId: string; readonly?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();
const stage = useAdeStageDialogStore();
const { data } = useHandovers(() => props.taskId);
const update = useUpdateHandover();

const drafts = computed(() => (data.value?.handovers ?? []).filter((h) => h.state === 'draft'));
const handed = computed(() => (data.value?.handovers ?? []).filter((h) => h.state === 'handed'));

const editing = ref<string | null>(null);
const text = ref('');
function edit(h: Handover): void {
  editing.value = h.id;
  text.value = h.body;
}
const saving = useAsyncAction(async () => {
  if (!editing.value) return;
  await update.mutateAsync({ id: editing.value, body: text.value });
  editing.value = null;
});

function stepLabel(name: string, id: string): string {
  return name || (id === 'done' ? t('common.done.status') : id);
}
function title(h: Handover): string {
  const from = stepLabel(h.fromStepName, h.fromStepId);
  return h.state === 'draft'
    ? t('space.ade.handover.draftFrom.label', { from })
    : t('space.ade.handover.fromTo.label', { from, to: stepLabel(h.toStepName, h.toStepId) });
}
const SOURCE = {
  agent: 'memory.common.origin.agent.label',
  run: 'common.action.run.label',
  user: 'common.picker.you.label',
} as const;
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="ade-handovers">
    <div class="text-kira-sm text-muted-foreground">{{ t('space.ade.handover.title.title') }}</div>
    <div
      v-if="!drafts.length && !handed.length"
      class="text-kira-sm text-subtle"
      data-testid="ade-handover-empty"
    >
      {{ t('space.ade.handover.empty.body') }}
    </div>
    <Card
      v-for="h in [...drafts, ...handed]"
      :key="h.id"
      class="flex flex-col gap-1 p-2 text-kira-md"
      :data-testid="h.state === 'draft' ? 'ade-handover-draft' : 'ade-handover-row'"
    >
      <div class="flex items-center gap-2 text-kira-sm">
        <span class="font-medium" data-testid="ade-handover-title">{{ title(h) }}</span>
        <AdeChip :label="t(SOURCE[h.source])" tone="grey" />
        <AdeChip v-if="h.state === 'draft'" :label="t('common.draft.status')" tone="amber" />
        <AdeChip
          v-if="h.state === 'draft' && h.finished"
          :label="t('space.ade.handover.finished.status')"
          tone="green"
        />
        <AdeChip v-if="h.edited" :label="t('llmPrompts.badge.edited.label')" tone="grey" />
        <span class="ml-auto text-subtle">{{ formatRelative(h.updatedAt) }}</span>
        <AdeOriginSession
          :task-id="taskId"
          :session-id="h.sessionId"
          :archived="readonly"
          @open="emit('openSession', $event)"
        />
        <TooltipIconButton
          v-if="!readonly && editing !== h.id"
          icon="edit"
          :label="t('space.ade.handover.edit.tooltip')"
          data-testid="ade-handover-edit"
          @click="edit(h)"
        />
      </div>
      <template v-if="editing === h.id">
        <Textarea v-model="text" class="max-h-60 resize-y" data-testid="ade-handover-text" />
        <Alert v-if="saving.error.value" variant="error" data-testid="ade-handover-error"
          ><AlertDescription>{{ saving.error.value }}</AlertDescription></Alert
        >
        <div class="flex justify-end gap-2">
          <Button
            variant="dialog"
            size="kira"
            data-testid="ade-handover-cancel"
            @click="editing = null"
            >{{
              t('common.action.cancel.label')
            }}</Button
          >
          <Button
            variant="dialog-primary"
            size="kira"
            :pending="saving.pending.value"
            data-testid="ade-handover-save"
            @click="saving.run"
            >{{
              t('common.action.save.label')
            }}</Button
          >
        </div>
      </template>
      <div v-else class="whitespace-pre-wrap break-words" data-testid="ade-handover-body">{{
        h.body
      }}</div>
      <div v-if="h.state === 'draft' && !readonly && editing !== h.id" class="flex justify-end">
        <Button
          variant="dialog"
          size="kira"
          data-testid="ade-handover-move-on"
          @click="stage.open({ taskId })"
          >{{
            t('space.ade.handover.moveOn.label')
          }}</Button
        >
      </div>
    </Card>
  </div>
</template>
