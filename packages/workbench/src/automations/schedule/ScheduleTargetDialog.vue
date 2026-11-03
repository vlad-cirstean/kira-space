<script setup lang="ts">
import type { CustomScript } from '@shared/domain/scripts';
import { useQuery } from '@tanstack/vue-query';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import { useAsyncAction } from '../../behaviour/asyncAction';
import { useAutomationsModule } from '../module';
import AdeContextFields from '../run/AdeContextFields.vue';

// Kira Space: the ADE task and branch a recurring run targets. App state, not file text, so it saves
// apart from the run file.
const props = defineProps<{ script: CustomScript }>();
const emit = defineEmits<{ close: [] }>();
const ctx = useAutomationsModule();

const taskId = ref(props.script.schedule?.taskId ?? '');
const branchId = ref(props.script.schedule?.branchId ?? '');

const preview = useQuery({
  queryKey: computed(() => ['scheduleTarget', props.script.id, taskId.value, branchId.value]),
  queryFn: () =>
    ctx.runs.preview({
      scriptId: props.script.id,
      params: {},
      prompt: null,
      taskId: taskId.value,
      branchId: branchId.value,
      listTasks: true,
    }),
  staleTime: 0,
});
const pv = computed(() => preview.data.value ?? null);

const saving = useAsyncAction(async () => {
  await ctx.scripts.setScheduleTarget?.(props.script.id, taskId.value, branchId.value);
});
async function save(): Promise<void> {
  if (await saving.run()) emit('close');
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent size="md" data-testid="schedule-target-dialog">
      <DialogHeader close-testid="schedule-target-close">
        <DialogTitle>{{ t('automations.target.title', { name: script.name }) }}</DialogTitle>
      </DialogHeader>
      <DialogBody class="flex flex-col gap-3">
        <FieldDescription>{{ t('automations.target.hint') }}</FieldDescription>
        <AdeContextFields
          v-if="pv"
          v-model:task-id="taskId"
          v-model:branch-id="branchId"
          :needs="pv.needs"
          :ade="pv.ade"
        />
        <Alert v-if="saving.error.value" variant="error" data-testid="schedule-target-error">
          <AlertDescription>{{ saving.error.value }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <DialogClose as-child>
          <Button variant="dialog" size="kira-lg" data-testid="schedule-target-cancel">{{
            t('common.action.cancel.label')
          }}</Button>
        </DialogClose>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="saving.pending.value"
          data-testid="schedule-target-save"
          @click="save"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
