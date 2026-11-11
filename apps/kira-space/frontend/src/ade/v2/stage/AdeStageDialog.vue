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
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeLinkStrip from '../links/AdeLinkStrip.vue';
import AdeNotesEditor from '../notes/AdeNotesEditor.vue';
import { useAdeStageDialogStore } from '../state/adeStageDialog';

// Moves a task to another stage with handover notes for it. `routed` picks which mount shows it: the
// ADE shell for board actions, the prompt host for the routed `handover` popup.
const props = defineProps<{ routed: boolean }>();
const emit = defineEmits<{ close: []; notNow: [] }>();
const store = useAdeStageDialogStore();

const open = computed(() => store.isOpen && store.routed === props.routed);
const pass = computed(() => store.mode === 'pass');
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
          t(pass ? 'space.ade.stageDialog.passTitle.title' : 'space.ade.stageDialog.title.title', {
            task: store.card?.title ?? '',
          })
        }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <div class="flex items-center gap-2">
          <span v-if="!pass" class="text-muted-foreground" data-testid="ade-stage-from">{{
            t('space.ade.stageDialog.from.label', { stage: fromName })
          }}</span>
          <Label for="ade-stage-target" class="ml-auto text-muted-foreground">{{
            t('space.ade.stageDialog.target.label')
          }}</Label>
          <span v-if="pass" class="font-medium" data-testid="ade-stage-pass-target">{{
            store.passStep?.name ?? ''
          }}</span>
          <NativeSelect
            v-else
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
            t(
              pass
                ? 'space.ade.stageDialog.passMessage.label'
                : 'space.ade.stageDialog.notes.label',
            )
          }}</Label>
          <AdeNotesEditor
            id="ade-stage-notes"
            v-model="store.notes"
            compact
            :hard-breaks="store.draft?.source !== 'user'"
            :label="
              t(
                pass
                  ? 'space.ade.stageDialog.passMessage.label'
                  : 'space.ade.stageDialog.notes.label',
              )
            "
            :placeholder="
              t(
                pass
                  ? 'space.ade.stageDialog.passMessage.placeholder'
                  : 'space.ade.stageDialog.notes.placeholder',
              )
            "
            data-testid="ade-stage-notes"
          />
          <FieldDescription>{{
            t(pass ? 'space.ade.stageDialog.passMessage.hint' : 'space.ade.stageDialog.notes.hint')
          }}</FieldDescription>
        </div>
        <div v-if="store.card" data-testid="ade-stage-links">
          <AdeLinkStrip
            :task="store.card.task"
            mode="add"
            :branches="!pass && store.toStepId === 'done'"
          />
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
            pass ? t('space.ade.board.taskAction.nextPass.label') : t('space.common.move.label')
          }}</Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
