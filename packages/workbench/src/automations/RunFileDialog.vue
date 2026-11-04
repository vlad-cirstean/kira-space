<script setup lang="ts">
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
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
import { t } from '@theme/i18n';
import CodeTextEditor from '@workbench/editor/CodeTextEditor.vue';
import { shortcutKeydown } from '@workbench/shortcuts/useShortcut';
import { computed, ref, watch } from 'vue';
import { errorText } from '../bridge/codedError';
import ConfigFileBar from '../config/ConfigFileBar.vue';
import { useAutomationsModule } from './module';
import RunSecretsSection from './RunSecretsSection.vue';
import { useRunFile, useSaveRunFile } from './runFileQueries';

// One run file as text. The draft is the source while it has unsaved edits, so a push never resets
// it. Save sends the hash of the text the draft was edited from; a file changed elsewhere is
// refused with Reload and Overwrite.
const props = defineProps<{ file: string }>();
const emit = defineEmits<{ close: [] }>();

const ctx = useAutomationsModule();
const query = useRunFile(() => props.file);
const save = useSaveRunFile();

const draft = ref('');
const dirty = ref(false);
const baseHash = ref('');
const saveError = ref('');
const conflict = ref(false);

const view = computed(() => query.data.value);
const script = computed(() => ctx.scripts.records().find((s) => s.id === view.value?.id));
const title = computed(() => script.value?.name ?? props.file);

watch(
  view,
  (v) => {
    if (!v || dirty.value) return;
    draft.value = v.text;
    baseHash.value = v.hash;
  },
  { immediate: true },
);

// A change on disk refreshes the file while no edits are pending.
watch(
  () =>
    ctx.scripts
      .records()
      .map((s) => s.updatedAt)
      .join('|') + ctx.scripts.problems().length,
  () => {
    if (!dirty.value) void query.refetch();
  },
);

async function saveNow(): Promise<void> {
  if (!dirty.value) return;
  const text = draft.value;
  try {
    const next = await save.mutateAsync({ file: props.file, text, baseHash: baseHash.value });
    saveError.value = '';
    conflict.value = false;
    baseHash.value = next.hash;
    if (draft.value === text) dirty.value = false;
  } catch (err) {
    saveError.value = errorText(err);
    conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
  }
}

function resetTo(v: { text: string; hash: string } | undefined): void {
  draft.value = v?.text ?? '';
  baseHash.value = v?.hash ?? '';
  dirty.value = false;
  saveError.value = '';
  conflict.value = false;
}

async function reload(): Promise<void> {
  resetTo((await query.refetch()).data);
}

async function overwrite(): Promise<void> {
  const fresh = (await query.refetch()).data;
  if (!fresh) return;
  baseHash.value = fresh.hash;
  conflict.value = false;
  await saveNow();
}

function onInput(v: string | number): void {
  draft.value = String(v);
  dirty.value = true;
}

const onSubmitKey = shortcutKeydown(['form.submit'], () => void saveNow());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && emit('close')">
    <DialogContent size="lg" data-testid="run-file-dialog" :dirty="dirty" @keydown="onSubmitKey">
      <DialogHeader close-testid="run-file-close">
        <DialogTitle>{{ title }}</DialogTitle>
      </DialogHeader>
      <DialogBody class="min-h-80">
        <div v-if="view" class="flex min-w-0 flex-col gap-3">
          <ConfigFileBar
            :path="view.path"
            :error="view.problem"
            :open="() => ctx.files.open(view!.file)"
            :reveal="() => ctx.files.reveal(view!.file)"
            :about="() => ctx.files.open('AGENTS.md')"
          />
          <div
            v-if="view.lastGood"
            class="text-kira-sm text-subtle"
            data-testid="run-file-last-good"
          >
            {{ t('automations.file.lastGood.hint') }}
          </div>
          <div
            v-if="!view.editable"
            class="text-kira-sm text-subtle"
            data-testid="run-file-read-only"
          >
            {{ t('space.ade.context.readOnly.hint') }}
          </div>
          <template v-else>
            <CodeTextEditor
              :model-value="draft"
              language="yaml"
              :ariaLabel="t('automations.file.field.aria')"
              class="h-64"
              data-testid="run-file-text"
              @update:model-value="onInput"
              @save="saveNow"
              @submit="saveNow"
            />
          </template>
          <Alert v-if="saveError" variant="error" data-testid="run-file-save-error">
            <AlertDescription>{{ saveError }}</AlertDescription>
            <AlertAction v-if="conflict">
              <Button variant="link" size="inline" data-testid="run-file-reload" @click="reload">
                {{ t('common.action.refresh.label') }}
              </Button>
              <Button
                variant="link"
                size="inline"
                data-testid="run-file-overwrite"
                @click="overwrite"
              >
                {{ t('space.ade.workflows.conflict.overwrite.label') }}
              </Button>
            </AlertAction>
          </Alert>
          <RunSecretsSection v-if="script" :script="script" />
        </div>
        <div
          v-else-if="query.error.value"
          class="text-kira-sm text-tone-red"
          data-testid="run-file-error"
        >
          {{ errorText(query.error.value) }}
        </div>
      </DialogBody>
      <DialogFooter>
        <DialogClose as-child>
          <Button variant="dialog" size="kira-lg" data-testid="run-file-cancel">{{
            dirty ? t('common.action.discard.label') : t('common.action.close.label')
          }}</Button>
        </DialogClose>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!dirty"
          :pending="save.isPending.value"
          data-testid="run-file-save"
          @click="saveNow"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
