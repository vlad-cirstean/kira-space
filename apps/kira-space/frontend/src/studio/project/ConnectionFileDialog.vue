<script setup lang="ts">
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { joinSentences, t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { useTaskAction } from '@workbench/behaviour/asyncAction';
import ConfigFileDialog from '@workbench/config/ConfigFileDialog.vue';
import { configName } from '@workbench/config/configName';
import { useConfigFormDraft } from '@workbench/config/useConfigFormDraft';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref, watch } from 'vue';
import {
  useConnectionConfig,
  useConnectionFile,
  useConnectionForm,
  useDeleteConnectionFile,
  useSaveConnectionFile,
  useSetConnectionSecret,
  useTestConnection,
} from '../state/connectionConfig';
import { useConnectionDialogStore } from '../state/connectionDialogs';
import { useConnectionsStore } from '../state/connections';
import ConnectionAdvancedTab from './connection/ConnectionAdvancedTab.vue';
import ConnectionGeneralTab from './connection/ConnectionGeneralTab.vue';
import ConnectionMcpTab from './connection/ConnectionMcpTab.vue';
import ConnectionPreconnectTab from './connection/ConnectionPreconnectTab.vue';
import ConnectionPrivacyTab from './connection/ConnectionPrivacyTab.vue';
import {
  CONNECTION_FIELDS,
  CONNECTION_TAB_POINTERS,
  type ConnectionSection,
  secretNameOf,
} from './connection/form';

// One connection's file as a form and as YAML over one draft. Save sends the hash of the text the
// draft was edited from; a file changed elsewhere is refused with Reload and Overwrite. A typed
// password or URI goes through the write-only secret call after the file is written.

const dialogStore = useConnectionDialogStore();
const connectionsStore = useConnectionsStore();
const config = useConnectionConfig();
const query = useConnectionFile(() => dialogStore.file);
const save = useSaveConnectionFile();
const test = useTestConnection();
const setSecret = useSetConnectionSecret();
const deleteFile = useDeleteConnectionFile();
const confirmDialogStore = useConfirmDialogStore();
const applyForm = useConnectionForm(() => dialogStore.file ?? '');

const testResult = ref<{ ok: boolean; message: string } | null>(null);
const secrets = ref({ password: '', uri: '' });

const detail = computed(() => query.data.value);
const record = computed(() => connectionsStore.connectionRecord(detail.value?.id));
const storageAvailable = computed(() => config.data.value?.storage.available ?? false);
const stem = computed(() => (detail.value?.file ?? '').replace(/\.yaml$/, ''));

async function flushSecrets(): Promise<void> {
  const model = draft.model.value;
  if (!model) return;
  for (const kind of ['password', 'uri'] as const) {
    const value = secrets.value[kind];
    const name = secretNameOf(model[kind]);
    if (!value || !name) continue;
    try {
      await setSecret.mutateAsync({ kind, name, value });
    } finally {
      setSecret.reset();
    }
    secrets.value = { ...secrets.value, [kind]: '' };
  }
}

const lifecycle = useTaskAction();

const draft = useConfigFormDraft<ConnectionFileModel>({
  file: () => (detail.value ? { text: detail.value.text, hash: detail.value.hash } : undefined),
  form: applyForm,
  save: (text, baseHash) => save.mutateAsync({ file: detail.value?.file ?? '', text, baseHash }),
  refetch: async () => {
    const fresh = (await query.refetch()).data;
    return fresh ? { text: fresh.text, hash: fresh.hash } : undefined;
  },
  fields: CONNECTION_FIELDS,
  secrets: { pending: () => hasTyped.value, flush: flushSecrets },
});

const section = ref<ConnectionSection>('general');
watch(
  () => dialogStore.file,
  () => {
    testResult.value = null;
    secrets.value = { password: '', uri: '' };
    section.value = 'general';
  },
);
// Declared after the file watch: a requested section wins over the reset.
watch(
  () => dialogStore.section,
  (s) => {
    if (!s) return;
    section.value = s;
    dialogStore.section = null;
  },
  { immediate: true },
);

const SECTIONS: ConnectionSection[] = [
  'general',
  'advanced',
  'preconnect',
  'mcp',
  'privacy',
  'yaml',
];
const LABEL = {
  general: () => t('studio.project.connectionDialog.tab.general.label'),
  advanced: () => t('common.advanced.label'),
  preconnect: () => t('studio.project.connectionDialog.tab.preconnect.label'),
  mcp: () => t('common.mcp.label'),
  privacy: () => t('studio.project.connectionDialog.tab.privacy.label'),
  yaml: () => t('common.yaml.label'),
} as const satisfies Record<ConnectionSection, () => string>;

const items = computed(() =>
  SECTIONS.map((id) => ({
    value: id,
    label: LABEL[id](),
    testid: `connection-tab-${id}`,
  })),
);

const model = computed(() => draft.model.value);
const savedSecret = (kind: 'password' | 'uri'): boolean => {
  const name = secretNameOf(model.value?.[kind]);
  return !!name && !!detail.value?.secrets.some((s) => s.kind === kind && s.name === name && s.set);
};
const hasTyped = computed(() => !!secrets.value.password || !!secrets.value.uri);

function close(): void {
  dialogStore.closeFile();
}

// The copy is a new file; the dialog follows it.
async function onDuplicate(): Promise<void> {
  const id = detail.value?.id;
  if (!id) return;
  await lifecycle.run(async () => {
    const created = await connectionsStore.duplicateConnection(id);
    dialogStore.openFile(created.file);
  });
}

async function onDelete(): Promise<void> {
  const d = detail.value;
  if (!d) return;
  const ok = await confirmDialogStore.confirm({
    title: t('studio.project.deleteConnectionConfirm.title'),
    message: joinSentences(
      t('studio.project.deleteConnectionConfirm.body', {
        name: record.value?.name ?? configName(d.file),
      }),
      t('common.confirm.irreversible.body'),
    ),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!ok) return;
  await lifecycle.run(async () => {
    if (d.id) await connectionsStore.deleteConnection(d.id);
    else await deleteFile.mutateAsync(d.file);
    close();
  });
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
</script>

<template>
  <ConfigFileDialog
    v-model:section="section"
    :draft="draft"
    :query="query"
    :title="
      t('studio.connections.file.title', {
        name: record?.name ?? configName(detail?.file ?? ''),
      })
    "
    testid-prefix="connection-file"
    :sections="items"
    :tabs-aria="t('studio.project.connectionDialog.tabs.aria')"
    :yaml-aria="t('studio.connections.file.field.aria')"
    :pointers="CONNECTION_TAB_POINTERS"
    :last-good="detail?.lastGood"
    :last-good-hint="t('studio.connections.file.lastGood.hint')"
    :problem="detail?.problem"
    :action-error="lifecycle.error.value"
    @close="close"
  >
    <template #form>
      <template v-if="detail">
        <ConnectionGeneralTab
          v-show="section === 'general'"
          v-model="draft.model.value!"
          v-model:secrets="secrets"
          :error-for="draft.errorFor"
          :stem="stem"
          :saved-password="savedSecret('password')"
          :saved-uri="savedSecret('uri')"
          :storage-available="storageAvailable"
          :connection-id="detail.id"
          :connection-name="record?.name ?? configName(detail.file)"
        />
        <ConnectionAdvancedTab
          v-show="section === 'advanced'"
          v-model="draft.model.value!"
          :error-for="draft.errorFor"
        />
        <ConnectionPreconnectTab
          v-show="section === 'preconnect'"
          v-model="draft.model.value!"
          :error-for="draft.errorFor"
        />
        <ConnectionMcpTab
          v-show="section === 'mcp'"
          v-model="draft.model.value!"
          :connection-id="detail.id"
          :error-for="draft.errorFor"
        />
        <ConnectionPrivacyTab
          v-if="section === 'privacy'"
          v-model="draft.model.value!"
          :connection-id="detail.id"
          :error-for="draft.errorFor"
        />
      </template>
    </template>
    <template #footer-start>
      <Button
        variant="dialog"
        size="kira-lg"
        :disabled="!detail?.id || draft.dirty.value"
        data-testid="connection-duplicate"
        @click="onDuplicate"
      >
        <CodiconIcon :name="ICON.duplicate" :size="13" />
        {{ t('common.action.duplicate.label') }}
      </Button>
      <!-- conventions-allow N17: second footer action; one emphasised button rightmost (RD.2) -->
      <Button
        variant="dialog"
        size="kira-lg"
        :disabled="!detail"
        data-testid="connection-delete"
        @click="onDelete"
      >
        <CodiconIcon name="trash" :size="13" />
        {{ t('common.action.delete.label') }}
      </Button>
      <span v-if="detail?.id && draft.dirty.value" data-testid="connection-save-first">{{
        t('studio.project.connectionDialog.saveFirst.hint')
      }}</span>
      <Button
        variant="dialog"
        size="kira-lg"
        :disabled="!detail?.id || draft.dirty.value"
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
  </ConfigFileDialog>
</template>
