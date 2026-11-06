<script setup lang="ts">
import type { ConnectionSecret } from '@shared/domain/connectionConfig';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { ref } from 'vue';
import { useDeleteConnectionSecret, useSetConnectionSecret } from '../../state/connectionConfig';
import { revealConnectionSecret } from '../state/connectionReveal';

// The secrets one connection file names. A value goes in once and is never shown unless the gated
// reveal (the eye on its field) asks for it; no call but Reveal returns it.
const props = defineProps<{
  connectionId: string;
  connectionName: string;
  secrets: ConnectionSecret[];
  storageAvailable: boolean;
}>();

const setSecret = useSetConnectionSecret();
const deleteSecret = useDeleteConnectionSecret();

const key = (row: ConnectionSecret): string => `${row.kind}:${row.name}`;

const drafts = ref<Record<string, string>>({});
const error = ref('');

async function save(row: ConnectionSecret): Promise<void> {
  const value = drafts.value[key(row)] ?? '';
  if (value === '') return;
  try {
    await setSecret.mutateAsync({ kind: row.kind, name: row.name, value });
    drafts.value[key(row)] = '';
    error.value = '';
  } catch (err) {
    error.value = errorText(err);
  }
}

async function clear(row: ConnectionSecret): Promise<void> {
  const ok = await useConfirmDialogStore().confirm({
    title: t('studio.connections.file.secrets.clearConfirm.title', { name: row.name }),
    message: t('studio.connections.file.secrets.clearConfirm.body'),
    confirmLabel: t('common.action.clear.label'),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteSecret.mutateAsync({ kind: row.kind, name: row.name });
    error.value = '';
  } catch (err) {
    error.value = errorText(err);
  }
}

async function reveal(row: ConnectionSecret): Promise<string | undefined> {
  const result = await revealConnectionSecret(
    props.connectionId,
    props.connectionName,
    row.kind === 'uri' ? 'uri' : 'fields',
    () => true,
  );
  if (!result) return undefined;
  if (result.outcome === 'error') {
    error.value = result.error;
    return undefined;
  }
  error.value = '';
  return (row.kind === 'uri' ? result.uri : result.password) ?? '';
}
</script>

<template>
  <section class="flex min-w-0 flex-col gap-2" data-testid="connection-secrets">
    <h3 class="m-0 text-kira-lg font-medium">{{ t('space.ade.settings.secrets.title') }}</h3>
    <Alert v-if="!storageAvailable" variant="warn" data-testid="connection-secrets-storage-off">
      <AlertDescription>{{
        t('studio.connections.file.secrets.storageOff.hint')
      }}</AlertDescription>
    </Alert>
    <FieldDescription v-if="secrets.length === 0" data-testid="connection-secrets-none">
      {{ t('studio.connections.file.secrets.none.hint') }}
    </FieldDescription>
    <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
      <li
        v-for="row in secrets"
        :key="key(row)"
        class="flex flex-col gap-1"
        :data-testid="`connection-secret-${row.name}`"
      >
        <div class="flex items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-data text-kira-sm">{{ row.name }}</span>
          <Badge data-testid="connection-secret-kind">{{
            row.kind === 'uri'
              ? t('studio.connections.file.secrets.kindUri.label')
              : t('studio.project.connectionDialog.password.label')
          }}</Badge>
          <Badge :variant="row.set ? 'ok' : 'warn'" data-testid="connection-secret-state">{{
            row.set ? t('space.ade.settings.secrets.set.status') : t('docker.edit.unset.status')
          }}</Badge>
        </div>
        <div class="flex items-center gap-2">
          <SecretInput
            v-model="drafts[key(row)]"
            :set="row.set"
            :reveal="() => reveal(row)"
            class="min-w-0 flex-1"
            :disabled="!storageAvailable"
            :aria-label="t('studio.connections.file.secrets.value.aria', { name: row.name })"
            data-testid="connection-secret-value"
            @keydown.enter.prevent="save(row)"
          />
          <Button
            variant="dialog"
            size="kira-lg"
            :disabled="!storageAvailable || !drafts[key(row)]"
            :pending="setSecret.isPending.value"
            data-testid="connection-secret-save"
            @click="save(row)"
          >
            {{ t('common.action.save.label') }}
          </Button>
          <Button
            v-if="row.set"
            variant="dialog"
            size="kira-lg"
            data-testid="connection-secret-clear"
            @click="clear(row)"
          >
            {{ t('common.action.clear.label') }}
          </Button>
        </div>
      </li>
    </ul>
    <Alert v-if="error" variant="error" data-testid="connection-secret-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>
  </section>
</template>
