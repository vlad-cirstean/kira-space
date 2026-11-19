<script setup lang="ts">
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
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
import { Field, FieldDescription, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { ref } from 'vue';
import { useDeleteAgentSecret, useSetAgentSecret } from './queries';
import type { AgentSecretView, AgentSettingsView } from './types';

// The credentials the file references. A value goes in once and is never shown: the field clears when
// the dialog closes, and no call returns it.
defineProps<{ view: AgentSettingsView }>();

const setSecret = useSetAgentSecret();
const deleteSecret = useDeleteAgentSecret();

const target = ref<string | null>(null);
const value = ref('');
const error = ref('');

function open(row: AgentSecretView): void {
  target.value = row.name;
  value.value = '';
  error.value = '';
}

function close(): void {
  target.value = null;
  value.value = '';
}

async function submit(): Promise<void> {
  if (target.value === null || value.value === '') return;
  try {
    await setSecret.mutateAsync({ name: target.value, value: value.value });
    close();
  } catch (err) {
    error.value = errorText(err);
  }
}

async function remove(row: AgentSecretView): Promise<void> {
  const ok = await useConfirmDialogStore().confirm({
    title: t('common.configSecrets.deleteConfirm.title', { name: row.name }),
    message: t('space.ade.settings.secrets.deleteConfirm.body'),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteSecret.mutateAsync(row.name);
    error.value = '';
  } catch (err) {
    error.value = errorText(err);
  }
}

const refExample = '$' + '{secret:name}';
</script>

<template>
  <section class="flex min-w-0 flex-col gap-2" data-testid="ade-settings-secrets">
    <SectionHeading flush :label="t('common.configSecrets.title')" />
    <FieldDescription>
      {{ t('space.ade.settings.secrets.hint', { ref: refExample }) }}
    </FieldDescription>
    <Alert v-if="!view.storage.available" variant="warn" data-testid="ade-settings-storage-off">
      <AlertDescription>{{
        view.storage.reason ?? t('space.ade.settings.secrets.storageOff.hint')
      }}</AlertDescription>
    </Alert>
    <InlineState
      kind="empty"
      v-if="view.secrets.length === 0"
      data-testid="ade-settings-secrets-none"
    >
      {{ t('space.ade.settings.secrets.none.hint') }}
    </InlineState>
    <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
      <li
        v-for="row in view.secrets"
        :key="row.name"
        class="flex items-center gap-2"
        :data-testid="`ade-settings-secret-${row.name}`"
      >
        <span class="min-w-0 flex-1 truncate font-data text-kira-sm">{{ row.name }}</span>
        <Badge v-if="!row.referenced" data-testid="ade-settings-secret-unused">{{
          t('common.configSecrets.unused.status')
        }}</Badge>
        <Badge :variant="row.set ? 'ok' : 'warn'" data-testid="ade-settings-secret-state">{{
          row.set ? t('common.configSecrets.set.status') : t('docker.edit.unset.status')
        }}</Badge>
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!view.storage.available"
          data-testid="ade-settings-secret-set"
          @click="open(row)"
        >
          {{ t('common.configSecrets.set.label') }}
        </Button>
        <Button
          v-if="row.set"
          variant="dialog-danger"
          size="kira-lg"
          data-testid="ade-settings-secret-delete"
          @click="remove(row)"
        >
          {{ t('common.action.delete.label') }}
        </Button>
      </li>
    </ul>
    <Alert v-if="error" variant="error" data-testid="ade-settings-secret-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <Dialog :open="target !== null" @update:open="(v) => !v && close()">
      <DialogContent size="sm" :dirty="value !== ''" data-testid="ade-settings-secret-dialog">
        <DialogHeader>
          <DialogTitle>{{
            t('common.configSecrets.dialog.title', { name: target ?? '' })
          }}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <Field>
            <FieldLabel for="ade-settings-secret-value">{{
              t('common.field.value.label')
            }}</FieldLabel>
            <SecretInput
              id="ade-settings-secret-value"
              v-model="value"
              data-testid="ade-settings-secret-value"
              @keydown.enter.prevent="submit"
            />
          </Field>
          <FieldDescription>
            {{ t('common.configSecrets.dialog.hint') }}
          </FieldDescription>
          <Alert v-if="error" variant="error">
            <AlertDescription>{{ error }}</AlertDescription>
          </Alert>
        </DialogBody>
        <DialogFooter>
          <Button variant="dialog" size="kira-lg" @click="close">
            {{ t('common.action.cancel.label') }}
          </Button>
          <Button
            variant="dialog-primary"
            size="kira-lg"
            :disabled="value === ''"
            :pending="setSecret.isPending.value"
            data-testid="ade-settings-secret-save"
            @click="submit"
          >
            {{ t('common.action.save.label') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>
