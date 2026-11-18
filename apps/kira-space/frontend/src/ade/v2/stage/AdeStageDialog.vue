<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
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
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { Kbd } from '@theme/components/ui/kbd';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { formatShortcut } from '@workbench/shortcuts/keys';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, onBeforeUnmount, onMounted } from 'vue';
import AdeLinkStrip from '../links/AdeLinkStrip.vue';
import { AdeNotesEditor } from '../notes/asyncEditor';
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

const closeItems = computed(() => [
  {
    value: 'close',
    label: t('space.ade.stageDialog.sessions.close.label', { count: store.closable.length }),
    testid: 'ade-stage-close',
  },
  {
    value: 'keep',
    label: t('space.ade.stageDialog.sessions.keep.label'),
    testid: 'ade-stage-keep',
  },
]);

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
function move(): void {
  if (store.submit()) emit('close');
}
// The routed popup answers to two-key chords only; the board's own dialog keeps its keys.
const onMoveKey = shortcutKeydown(['form.submit'], () => {
  if (props.routed && !live.value && store.toStepId) move();
});
// The non-routed mount is the only one that can show a reopened (refused) move.
onMounted(() => {
  if (!props.routed) store.boardMounted = true;
});
onBeforeUnmount(() => {
  if (!props.routed) store.boardMounted = false;
});
</script>

<template>
  <Dialog :open="open" @update:open="(v) => !v && close()">
    <DialogContent
      size="lg"
      :prompt="routed"
      :dirty="store.dirty"
      data-testid="ade-stage-dialog"
      @keydown="onMoveKey"
    >
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
          <FieldLabel for="ade-stage-target" class="ml-auto">{{
            t('space.ade.stageDialog.target.label')
          }}</FieldLabel>
          <span v-if="pass" class="font-medium" data-testid="ade-stage-pass-target">{{
            store.passStep?.name ?? ''
          }}</span>
          <NativeSelect
            v-else
            id="ade-stage-target"
            v-model="store.toStepId"
            variant="bordered"
            data-testid="ade-stage-target"
          >
            <option v-for="o in targets" :key="o.id" :value="o.id">{{ o.name }}</option>
          </NativeSelect>
        </div>
        <Field>
          <FieldLabel for="ade-stage-notes">{{
            t(
              pass
                ? 'space.ade.stageDialog.passMessage.label'
                : 'space.ade.stageDialog.notes.label',
            )
          }}</FieldLabel>
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
        </Field>
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
          <SecondaryTabs
            variant="choice"
            size="kira-lg"
            :aria-label="
              t('space.ade.stageDialog.sessions.label', { count: store.closable.length })
            "
            :items="closeItems"
            :model-value="store.closeChoice"
            data-testid="ade-stage-close-choice"
            @update:model-value="(v) => (store.closeChoice = v === 'keep' ? 'keep' : 'close')"
          />
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
          :disabled="live || !store.toStepId"
          :aria-keyshortcuts="routed ? formatShortcut('form.submit') : undefined"
          data-testid="ade-stage-move"
          @click="move"
          >{{ pass ? t('space.ade.board.taskAction.nextPass.label') : t('space.common.move.label')
          }}<Kbd v-if="routed">{{ formatShortcut('form.submit') }}</Kbd></Button
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
