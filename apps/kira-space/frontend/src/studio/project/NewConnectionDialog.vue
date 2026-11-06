<script setup lang="ts">
import type { ConnectionInput, ConnectionKind } from '@shared/domain/connection';
import { AWS_STYLE_KINDS, DEFAULT_PORT, FILE_KINDS } from '@shared/domain/connection';
import CodiconIcon from '@theme/CodiconIcon.vue';
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
import { Field, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { computed, ref } from 'vue';
import { control } from '../bridge/control';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useConnectionsStore } from '../state/connections';
import ConnectionEnginePicker from './connection/ConnectionEnginePicker.vue';
import { KIND_LABEL, SUPPORTED_KINDS } from './connection/kinds';

// A new connection: pick the engine, name it, and the file is written with the engine's defaults.
// Everything else is edited in the file, which opens next.
const dialogStore = useConnectionDialogStore();
const connectionsStore = useConnectionsStore();

const kind = ref<ConnectionKind>('postgres');
const step = ref<'engine' | 'details'>('engine');
const name = ref('');
const database = ref('');

const isFileKind = computed(() => FILE_KINDS.has(kind.value));

function pickKind(picked: ConnectionKind): void {
  if (!SUPPORTED_KINDS.has(picked)) return;
  kind.value = picked;
  step.value = 'details';
}

function inputFor(): ConnectionInput {
  const k = kind.value;
  const aws = AWS_STYLE_KINDS.has(k);
  return {
    name: name.value.trim(),
    kind: k,
    color: 'none',
    mode: 'fields',
    readOnly: false,
    host: isFileKind.value || aws ? null : 'localhost',
    port: isFileKind.value || aws ? null : (DEFAULT_PORT[k] ?? null),
    database: isFileKind.value ? database.value.trim() : null,
    username: null,
    password: null,
    uri: null,
    options: {},
    preconnect: null,
    preconnectSidecar: false,
    preconnectDelaySec: 0,
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit', masked: 'inherit' },
    mcpMaskedMode: 'inherit',
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    maskReviewOn: false,
  };
}

// Guards a double click: two in-flight creates would write two files.
const {
  run: create,
  pending,
  error,
} = useAsyncAction(async () => {
  const created = await control.connectionsCreate(inputFor());
  connectionsStore.upsertRecord(created);
  dialogStore.closeNew();
  dialogStore.openFile(created.file);
  return created;
});

const canCreate = computed(
  () => name.value.trim() !== '' && (!isFileKind.value || database.value.trim() !== ''),
);

async function browse(): Promise<void> {
  const res = await control.filesChooseOpen({
    filters: [
      {
        name: t('studio.project.connectionDialog.sqliteFilter.label'),
        extensions: ['sqlite', 'sqlite3', 'db', 'db3'],
      },
      { name: t('studio.common.allFiles.label'), extensions: ['*'] },
    ],
    title: t('studio.project.connectionDialog.databasePicker.title'),
  });
  if (!res.canceled && res.file) database.value = res.file.path;
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && dialogStore.closeNew()">
    <DialogContent size="2xl" fixed-height :dirty="name !== ''" data-testid="new-connection-dialog">
      <DialogHeader close-testid="new-connection-close">
        <template v-if="step === 'details'">
          <DialogTitle>{{
            t('studio.project.connectionDialog.newKindTitle.title', { kind: KIND_LABEL[kind] })
          }}</DialogTitle>
          <span class="min-w-0 grow" />
          <Button variant="toolbar" size="kira" @click="step = 'engine'">
            <CodiconIcon name="chevron-left" :size="13" />
            {{ t('studio.project.connectionDialog.changeEngine.title') }}
          </Button>
        </template>
        <DialogTitle v-else>{{
          t('studio.project.connectionDialog.newConnection.title')
        }}</DialogTitle>
      </DialogHeader>

      <DialogBody flush>
        <ConnectionEnginePicker v-if="step === 'engine'" :kind="kind" @pick="pickKind" />
        <div v-else class="flex flex-col gap-3 p-3">
          <Field>
            <FieldLabel for="new-connection-name">{{ t('common.field.name.label') }}</FieldLabel>
            <Input
              id="new-connection-name"
              v-model="name"
              data-testid="new-connection-name"
              @keydown.enter.prevent="canCreate && create()"
            />
          </Field>
          <Field v-if="isFileKind">
            <FieldLabel for="new-connection-database">{{
              t('studio.project.connectionDialog.databaseFile.label')
            }}</FieldLabel>
            <div class="flex items-center gap-1">
              <Input
                id="new-connection-database"
                v-model="database"
                class="min-w-0 flex-1 font-data"
                data-testid="new-connection-database"
              />
              <Button
                variant="dialog"
                size="kira-lg"
                data-testid="new-connection-browse"
                @click="browse"
              >
                {{ t('studio.common.browse.label') }}
              </Button>
            </div>
          </Field>
          <Alert v-if="error" variant="error" data-testid="new-connection-error">
            <AlertDescription>{{ error }}</AlertDescription>
          </Alert>
        </div>
      </DialogBody>

      <DialogFooter>
        <Button
          variant="dialog"
          size="kira-lg"
          data-testid="new-connection-cancel"
          @click="dialogStore.closeNew"
        >
          {{ t('common.action.cancel.label') }}
        </Button>
        <Button
          v-if="step === 'details'"
          variant="dialog-primary"
          size="kira-lg"
          :disabled="!canCreate"
          :pending="pending"
          data-testid="new-connection-create"
          @click="create"
        >
          {{ t('common.action.create.label') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
