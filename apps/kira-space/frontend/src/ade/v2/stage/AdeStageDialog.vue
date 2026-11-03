<script setup lang="ts">
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeArtifactList from '../artifacts/AdeArtifactList.vue';
import { useAdeStageDialogStore } from '../state/adeStageDialog';

// Moves a task to another stage with handover notes for it. `routed` picks which mount shows it: the
// ADE shell for board actions, the prompt host for the routed `handover` popup.
const props = defineProps<{ routed: boolean }>();
const emit = defineEmits<{ close: []; notNow: [] }>();
const store = useAdeStageDialogStore();

const open = computed(() => store.isOpen && store.routed === props.routed);
const live = computed(() => store.moves?.live ?? false);
const fromName = computed(
  () => store.moves?.options.find((o) => o.id === store.fromStepId)?.name ?? '',
);
const targets = computed(() =>
  (store.moves?.options ?? []).filter((o) => o.id !== store.fromStepId && !o.skipped),
);

const sessionStages = computed(() => {
  const names = new Map((store.moves?.options ?? []).map((o) => [o.id, o.name]));
  const owners = store.closable.map((s) => (s.purpose === 'review' ? 'review' : s.stepId));
  return [...new Set(owners)]
    .map((id) => names.get(id) ?? '')
    .filter(Boolean)
    .join(', ');
});

function close(): void {
  store.close();
  emit('close');
}
async function move(): Promise<void> {
  if (await store.submit()) emit('close');
}
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && close()">
    <DialogContent size="lg" :dirty="store.dirty" data-testid="ade-stage-dialog">
      <DialogHeader>
        <DialogTitle>{{
          t('space.ade.stageDialog.title.title', { task: store.card?.title ?? '' })
        }}</DialogTitle>
      </DialogHeader>
      <DialogBody class="flex flex-col gap-3 text-kira-md">
        <div class="flex items-center gap-2">
          <span class="text-muted-foreground" data-testid="ade-stage-from">{{
            t('space.ade.stageDialog.from.label', { stage: fromName })
          }}</span>
          <Label for="ade-stage-target" class="ml-auto text-muted-foreground">{{
            t('space.ade.stageDialog.target.label')
          }}</Label>
          <NativeSelect
            id="ade-stage-target"
            v-model="store.toStepId"
            variant="bordered"
            size="kira-lg"
            data-testid="ade-stage-target"
          >
            <option v-for="o in targets" :key="o.id" :value="o.id">{{ o.name }}</option>
          </NativeSelect>
        </div>
        <div class="flex flex-col gap-1">
          <Label for="ade-stage-notes" class="text-muted-foreground">{{
            t('space.ade.stageDialog.notes.label')
          }}</Label>
          <Textarea
            id="ade-stage-notes"
            v-model="store.notes"
            class="max-h-90 resize-y"
            :placeholder="t('space.ade.stageDialog.notes.placeholder')"
            data-testid="ade-stage-notes"
          />
          <FieldDescription>{{ t('space.ade.stageDialog.notes.hint') }}</FieldDescription>
        </div>
        <div
          v-if="store.closable.length"
          class="flex flex-col gap-1.5"
          data-testid="ade-stage-sessions"
        >
          <div class="text-muted-foreground">
            {{ t('space.ade.stageDialog.sessions.label', { count: store.closable.length }) }}
            <span v-if="sessionStages" class="text-kira-sm">({{ sessionStages }})</span>
          </div>
          <RadioGroup v-model="store.closeChoice" class="gap-1.5">
            <div class="flex items-center gap-2">
              <RadioGroupItem id="ade-stage-close" value="close" data-testid="ade-stage-close" />
              <Label for="ade-stage-close">{{
                t('space.ade.stageDialog.sessions.close.label', { count: store.closable.length })
              }}</Label>
            </div>
            <div class="flex items-center gap-2">
              <RadioGroupItem id="ade-stage-keep" value="keep" data-testid="ade-stage-keep" />
              <Label for="ade-stage-keep">{{
                t('space.ade.stageDialog.sessions.keep.label')
              }}</Label>
            </div>
          </RadioGroup>
          <FieldDescription>{{ t('space.ade.stageDialog.sessions.hint') }}</FieldDescription>
        </div>
        <div
          v-if="store.taskId && store.toStepId === 'done'"
          class="flex flex-col gap-1"
          data-testid="ade-stage-artifacts"
        >
          <div class="text-muted-foreground">{{ t('space.ade.artifacts.stageDone.label') }}</div>
          <AdeArtifactList :task-id="store.taskId" />
        </div>
        <Alert v-if="live" variant="error" data-testid="ade-stage-live">
          <AlertDescription>{{
            t('space.ade.board.taskMenu.liveBlocked.tooltip')
          }}</AlertDescription>
        </Alert>
        <Alert v-if="store.error" variant="error" data-testid="ade-stage-error">
          <AlertDescription>{{ store.error }}</AlertDescription>
        </Alert>
      </DialogBody>
      <DialogFooter>
        <Button
          v-if="routed"
          variant="dialog"
          size="kira-lg"
          data-testid="ade-stage-not-now"
          @click="emit('notNow')"
          >{{
            t('git.dialog.postCheckoutPullConfirm.notNow.label')
          }}</Button
        >
        <Button variant="dialog" size="kira-lg" data-testid="ade-stage-cancel" @click="close">{{
          t('common.action.cancel.label')
        }}</Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :pending="store.pending"
          :disabled="live || !store.toStepId"
          data-testid="ade-stage-move"
          @click="move"
          >{{
            t('space.common.move.label')
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
