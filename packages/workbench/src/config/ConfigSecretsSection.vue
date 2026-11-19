<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
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
import { ICON } from '@theme/icons';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import SecretInput from '@workbench/components/SecretInput.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { ref } from 'vue';

export interface ConfigSecretRow {
  name: string;
  set: boolean;
  referenced: boolean;
}

export interface ConfigSecretStorage {
  available: boolean;
  reason?: string | null;
}

// The credentials a config file references, for any host (API collections, ADE settings). A value
// goes in once and is never shown: the field clears when the dialog closes, and no call returns it.
// `testidPrefix` is the singular id (`api-secret`): the section is `<prefix>s`, a row `<prefix>-<name>`.
// The `title` slot replaces the default SectionHeading and `description` the hint; `description`
// receives the reference syntax example as `ref`.
const props = defineProps<{
  secrets: readonly ConfigSecretRow[];
  storage: ConfigSecretStorage | null;
  setSecret: (name: string, value: string) => Promise<void>;
  deleteSecret: (name: string) => Promise<void>;
  testidPrefix: string;
}>();

const target = ref<string | null>(null);
const value = ref('');
const error = ref('');
const save = useAsyncAction((name: string, secret: string) => props.setSecret(name, secret));
const drop = useAsyncAction((name: string) => props.deleteSecret(name));

const tid = (suffix: string): string => `${props.testidPrefix}-${suffix}`;

function open(row: ConfigSecretRow): void {
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
  if (await save.run(target.value, value.value)) close();
  else error.value = save.error.value ?? '';
}

async function remove(row: ConfigSecretRow): Promise<void> {
  const ok = await useConfirmDialogStore().confirm({
    title: t('common.configSecrets.deleteConfirm.title', { name: row.name }),
    message: t('common.configSecrets.deleteConfirm.body'),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
  });
  if (!ok) return;
  error.value = (await drop.run(row.name)) ? '' : (drop.error.value ?? '');
}

const refExample = '$' + '{secret:name}';
</script>

<template>
  <section class="flex min-w-0 flex-col gap-2" :data-testid="`${testidPrefix}s`">
    <slot name="title"><SectionHeading flush :label="t('common.configSecrets.title')" /></slot>
    <FieldDescription>
      <slot name="description" :ref="refExample">{{
        t('common.configSecrets.hint', { ref: refExample })
      }}</slot>
    </FieldDescription>
    <Alert
      v-if="storage && !storage.available"
      variant="warn"
      :data-testid="`${testidPrefix}s-storage-off`"
    >
      <AlertDescription>{{
        storage.reason ?? t('common.configSecrets.storageOff.hint')
      }}</AlertDescription>
    </Alert>
    <InlineState v-if="secrets.length === 0" kind="empty" :data-testid="`${testidPrefix}s-none`">
      {{ t('common.configSecrets.none.hint') }}
    </InlineState>
    <ul v-else class="m-0 flex list-none flex-col gap-1 p-0">
      <li
        v-for="row in secrets"
        :key="row.name"
        class="flex items-center gap-2"
        :data-testid="tid(row.name)"
      >
        <span class="min-w-0 flex-1 truncate font-data text-kira-sm">{{ row.name }}</span>
        <Badge v-if="!row.referenced" :data-testid="tid('unused')">{{
          t('common.configSecrets.unused.status')
        }}</Badge>
        <Badge :variant="row.set ? 'ok' : 'warn'" :data-testid="tid('state')">{{
          row.set ? t('common.configSecrets.set.status') : t('common.unset.status')
        }}</Badge>
        <Button
          variant="toolbar"
          size="kira"
          :disabled="!storage?.available"
          :data-testid="tid('set')"
          @click="open(row)"
          ><CodiconIcon :name="ICON.edit" :size="13" />{{ t('common.configSecrets.set.label') }}
        </Button>
        <Button
          v-if="row.set"
          variant="toolbar"
          size="kira"
          data-tone="error"
          :data-testid="tid('delete')"
          @click="remove(row)"
          ><CodiconIcon :name="ICON.delete" :size="13" />{{ t('common.action.delete.label') }}
        </Button>
      </li>
    </ul>
    <Alert v-if="error && target === null" variant="error" :data-testid="tid('error')">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <Dialog :open="target !== null" @update:open="(v) => !v && close()">
      <DialogContent size="sm" :dirty="value !== ''" :data-testid="tid('dialog')">
        <DialogHeader>
          <DialogTitle>{{
            t('common.configSecrets.dialog.title', { name: target ?? '' })
          }}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <Field>
            <FieldLabel :for="tid('value')">{{ t('common.field.value.label') }}</FieldLabel>
            <SecretInput
              :id="tid('value')"
              v-model="value"
              :data-testid="tid('value')"
              @keydown.enter.prevent="submit"
            />
          </Field>
          <FieldDescription>{{ t('common.configSecrets.dialog.hint') }}</FieldDescription>
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
            :pending="save.pending.value"
            :data-testid="tid('save')"
            @click="submit"
          >
            {{ t('common.action.save.label') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>
