<script setup lang="ts">
import type { ApiSecret } from '@shared/domain/apiConfig';
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
import { computed, ref } from 'vue';
import { useApiConfig, useDeleteApiSecret, useSetApiSecret } from './state/apiConfig';

// The credentials the files reference. A value goes in once and is never shown: the field clears
// when the dialog closes, and no call returns it.
const config = useApiConfig();
const setSecret = useSetApiSecret();
const deleteSecret = useDeleteApiSecret();

const secrets = computed<ApiSecret[]>(() => config.data.value?.secrets ?? []);
const storage = computed(() => config.data.value?.storage ?? null);

const target = ref<string | null>(null);
const value = ref('');
const error = ref('');

function open(row: ApiSecret): void {
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

async function clear(row: ApiSecret): Promise<void> {
  const ok = await useConfirmDialogStore().confirm({
    title: t('studio.api.config.secrets.clearConfirm.title', { name: row.name }),
    message: t('studio.api.config.secrets.clearConfirm.body'),
    confirmLabel: t('studio.api.config.secrets.clear.label'),
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
  <section class="flex min-w-0 flex-col gap-2 p-2" data-testid="api-secrets">
    <h3 class="m-0 text-kira-lg font-medium">{{ t('studio.api.config.secrets.title') }}</h3>
    <FieldDescription>
      {{ t('studio.api.config.secrets.hint', { ref: refExample }) }}
    </FieldDescription>
    <Alert
      v-if="storage && !storage.available"
      variant="warn"
      data-testid="api-secrets-storage-off"
    >
      <AlertDescription>{{
        storage.reason ?? t('studio.api.config.secrets.storageOff.hint')
      }}</AlertDescription>
    </Alert>
    <FieldDescription v-if="secrets.length === 0" data-testid="api-secrets-none">
      {{ t('studio.api.config.secrets.none.hint') }}
    </FieldDescription>
    <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
      <li
        v-for="row in secrets"
        :key="row.name"
        class="flex items-center gap-2"
        :data-testid="`api-secret-${row.name}`"
      >
        <span class="min-w-0 flex-1 truncate font-data text-kira-sm">{{ row.name }}</span>
        <Badge v-if="!row.referenced" data-testid="api-secret-unused">{{
          t('studio.api.config.secrets.unused.status')
        }}</Badge>
        <Badge :variant="row.set ? 'ok' : 'warn'" data-testid="api-secret-state">{{
          row.set
            ? t('studio.api.config.secrets.set.status')
            : t('studio.api.config.secrets.unset.status')
        }}</Badge>
        <Button
          variant="dialog"
          size="kira-lg"
          :disabled="!storage?.available"
          data-testid="api-secret-set"
          @click="open(row)"
        >
          {{ t('studio.api.config.secrets.set.label') }}
        </Button>
        <Button
          v-if="row.set"
          variant="dialog"
          size="kira-lg"
          data-testid="api-secret-clear"
          @click="clear(row)"
        >
          {{ t('studio.api.config.secrets.clear.label') }}
        </Button>
      </li>
    </ul>
    <Alert v-if="error && target === null" variant="error" data-testid="api-secret-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <Dialog :open="target !== null" @update:open="(v) => !v && close()">
      <DialogContent size="sm" :dirty="value !== ''" data-testid="api-secret-dialog">
        <DialogHeader>
          <DialogTitle>{{
            t('studio.api.config.secrets.dialog.title', { name: target ?? '' })
          }}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <Field>
            <FieldLabel for="api-secret-value">{{ t('common.field.value.label') }}</FieldLabel>
            <SecretInput
              id="api-secret-value"
              v-model="value"
              data-testid="api-secret-value"
              @keydown.enter.prevent="submit"
            />
          </Field>
          <FieldDescription>
            {{ t('studio.api.config.secrets.dialog.hint') }}
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
            data-testid="api-secret-save"
            @click="submit"
          >
            {{ t('common.action.save.label') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>
