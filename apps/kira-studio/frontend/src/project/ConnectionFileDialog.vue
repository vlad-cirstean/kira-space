<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertAction, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { Label } from '@theme/components/ui/label';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import ConfigFileBar from '@workbench/config/ConfigFileBar.vue';
import { useShortcut } from '@workbench/shortcuts/useShortcut';
import { computed, ref, useTemplateRef, watch } from 'vue';
import { control } from '../bridge/control';
import {
  useConnectionConfig,
  useConnectionFile,
  useSaveConnectionFile,
  useTestConnection,
} from '../state/connectionConfig';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useConnectionsStore } from '../state/connections';
import ConnectionSecrets from './connection/ConnectionSecrets.vue';

// One connection's file as text. The draft is the source while it has unsaved edits, so a push never
// resets it. Save sends the hash of the text the draft was edited from; a file changed elsewhere is
// refused with Reload and Overwrite.
const FOLDER = 'connections';

const dialogStore = useConnectionDialogStore();
const connectionsStore = useConnectionsStore();
const config = useConnectionConfig();
const query = useConnectionFile(() => dialogStore.file);
const save = useSaveConnectionFile();
const test = useTestConnection();

const draft = ref('');
const dirty = ref(false);
const baseHash = ref('');
const saveError = ref('');
const conflict = ref(false);
const testResult = ref<{ ok: boolean; message: string } | null>(null);

const detail = computed(() => query.data.value);
const record = computed(() => connectionsStore.connectionRecord(detail.value?.id));
const storageAvailable = computed(() => config.data.value?.storage.available ?? false);

watch(
  detail,
  (d) => {
    if (!d || dirty.value) return;
    draft.value = d.text;
    baseHash.value = d.hash;
  },
  { immediate: true },
);

watch(
  () => dialogStore.file,
  () => {
    dirty.value = false;
    saveError.value = '';
    conflict.value = false;
    testResult.value = null;
  },
);

function close(): void {
  dialogStore.closeFile();
}

async function saveNow(): Promise<void> {
  const d = detail.value;
  if (!d || !dirty.value || save.isPending.value) return;
  const text = draft.value;
  try {
    const next = await save.mutateAsync({ file: d.file, text, baseHash: baseHash.value });
    saveError.value = '';
    conflict.value = false;
    baseHash.value = next.hash;
    if (draft.value === text) dirty.value = false;
  } catch (err) {
    saveError.value = errorText(err);
    conflict.value = (err as { code?: string }).code === 'E_CONFLICT';
  }
}

function resetTo(d: { text: string; hash: string } | undefined): void {
  draft.value = d?.text ?? '';
  baseHash.value = d?.hash ?? '';
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
  dirty.value = true;
  await saveNow();
}

function onInput(v: string | number): void {
  draft.value = String(v);
  dirty.value = true;
}

async function onTest(): Promise<void> {
  const id = detail.value?.id;
  if (!id) return;
  testResult.value = null;
  const result = await test.mutateAsync(id);
  testResult.value = result.ok
    ? { ok: true, message: result.serverVersion ?? '' }
    : { ok: false, message: result.error ?? '' };
}

const root = useTemplateRef<HTMLElement>('root');
useShortcut(root, ['form.save'], () => void saveNow());
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && close()">
    <DialogContent size="2xl" fixed-height :dirty="dirty" data-testid="connection-file-dialog">
      <DialogHeader close-testid="connection-file-close">
        <DialogTitle>{{
          t('studio.connections.file.title', { name: record?.name ?? detail?.file ?? '' })
        }}</DialogTitle>
      </DialogHeader>

      <DialogBody v-if="detail">
        <div ref="root" class="flex flex-col gap-3">
          <ConfigFileBar
            :path="detail.path"
            :error="detail.problem"
            :open="() => control.configFilesOpen({ folder: FOLDER, file: detail!.file })"
            :reveal="() => control.configFilesReveal({ folder: FOLDER, file: detail!.file })"
            :about="() => control.configFilesOpen({ folder: FOLDER, file: 'AGENTS.md' })"
          />
          <div
            v-if="detail.lastGood"
            class="text-kira-sm text-subtle"
            data-testid="connection-file-last-good"
          >
            {{ t('studio.connections.file.lastGood.hint') }}
          </div>
          <Label for="connection-file-text" class="sr-only">{{
            t('studio.connections.file.field.aria')
          }}</Label>
          <Textarea
            id="connection-file-text"
            :model-value="draft"
            spellcheck="false"
            class="min-h-72 resize-y font-data leading-relaxed"
            data-testid="connection-file-text"
            @update:model-value="onInput"
          />
          <Alert v-if="saveError" variant="error" data-testid="connection-file-save-error">
            <AlertDescription>{{ saveError }}</AlertDescription>
            <AlertAction v-if="conflict">
              <Button
                variant="link"
                size="inline"
                data-testid="connection-file-reload"
                @click="reload"
              >
                {{ t('common.action.refresh.label') }}
              </Button>
              <Button
                variant="link"
                size="inline"
                data-testid="connection-file-overwrite"
                @click="overwrite"
              >
                {{ t('space.ade.workflows.conflict.overwrite.label') }}
              </Button>
            </AlertAction>
          </Alert>
          <ConnectionSecrets
            v-if="detail.id"
            :connection-id="detail.id"
            :connection-name="record?.name ?? detail.file"
            :secrets="detail.secrets"
            :storage-available="storageAvailable"
          />
        </div>
      </DialogBody>

      <DialogFooter>
        <template #start>
          <Button
            variant="dialog"
            size="kira-lg"
            :disabled="!detail?.id || dirty"
            :pending="test.isPending.value"
            data-testid="connection-test"
            @click="onTest"
          >
            <CodiconIcon name="plug" :size="13" />
            {{ t('studio.project.connectionDialog.test.label') }}
          </Button>
          <Badge
            v-if="testResult"
            class="min-w-0"
            :variant="testResult.ok ? 'ok' : 'error'"
            data-testid="connection-test-result"
          >
            <TruncatedText :text="testResult.message" />
          </Badge>
        </template>
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!dirty"
          data-testid="connection-file-discard"
          @click="resetTo(detail)"
        >
          {{ t('common.action.discard.label') }}
        </Button>
        <Button
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!dirty"
          :pending="save.isPending.value"
          data-testid="connection-file-save"
          @click="saveNow"
        >
          {{ t('common.action.save.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
