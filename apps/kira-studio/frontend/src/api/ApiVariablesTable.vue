<script setup lang="ts">
import type { ApiFileVariable } from '@shared/domain/apiConfig';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Input } from '@theme/components/ui/input';
import { tableHeadClass, tableHeadRowClass } from '@theme/components/ui/table';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { cn } from '@theme/lib/utils';
import SecretInput from '@workbench/components/SecretInput.vue';
import { computed } from 'vue';
import { secretNameOf, secretRef, variableSecretName } from './variablesForm';

// The variable rows of one file: name, value or secret, description. A secret row holds the
// reference only; a value typed on it waits in `typed` for the write-only call on Save.
const props = defineProps<{
  /** File stem, for the name of a secret this table creates. */
  stem: string;
  /** Names of secrets that already hold a value. */
  savedSecrets: ReadonlySet<string>;
  storageAvailable: boolean;
}>();
const rows = defineModel<ApiFileVariable[]>({ required: true });
const typed = defineModel<Record<string, string>>('typed', { required: true });

const headCell = cn(tableHeadClass, 'px-0');
const duplicates = computed(() => {
  const seen = new Set<string>();
  const dup = new Set<string>();
  for (const r of rows.value) {
    if (seen.has(r.name)) dup.add(r.name);
    seen.add(r.name);
  }
  return dup;
});

function patch(i: number, change: Partial<ApiFileVariable>): void {
  rows.value = rows.value.map((r, j) => (j === i ? { ...r, ...change } : r));
}

function setSecret(i: number, on: boolean): void {
  const row = rows.value[i];
  if (!row) return;
  if (on) {
    patch(i, { secret: secretRef(variableSecretName(props.stem, row.name)), value: undefined });
    return;
  }
  const name = secretNameOf(row.secret);
  if (name) typed.value = { ...typed.value, [name]: '' };
  patch(i, { secret: undefined });
}

function setTyped(i: number, value: string): void {
  const name = secretNameOf(rows.value[i]?.secret);
  if (name) typed.value = { ...typed.value, [name]: value };
}

function add(): void {
  rows.value = [...rows.value, { name: '', value: '' }];
}

function remove(i: number): void {
  rows.value = rows.value.filter((_, j) => j !== i);
}
</script>

<template>
  <div class="flex flex-col gap-1" data-testid="api-variables-table">
    <div :class="cn(tableHeadRowClass, 'grid grid-cols-[1.2fr_2fr_auto_1.5fr_auto] gap-1 px-1.5')">
      <span :class="headCell">{{ t('common.field.name.label') }}</span>
      <span :class="headCell">{{ t('common.field.value.label') }}</span>
      <span :class="headCell">{{ t('common.secret.label') }}</span>
      <span :class="headCell">{{ t('common.field.description.label') }}</span>
      <span></span>
    </div>
    <div
      v-for="(row, i) in rows"
      :key="i"
      class="grid grid-cols-[1.2fr_2fr_auto_1.5fr_auto] items-center gap-1 px-1.5"
      :data-testid="`api-variable-${i}`"
    >
      <div class="flex min-w-0 items-center gap-1">
        <Input
          :model-value="row.name"
          :aria-label="t('studio.api.variablesDialog.name.aria')"
          class="font-data"
          :data-testid="`api-variable-name-${i}`"
          @update:model-value="(v) => patch(i, { name: String(v) })"
        />
        <Badge v-if="duplicates.has(row.name)" variant="warn">{{
          t('studio.api.variablesDialog.duplicate.status')
        }}</Badge>
      </div>
      <SecretInput
        v-if="row.secret"
        :model-value="typed[secretNameOf(row.secret)] ?? ''"
        :set="savedSecrets.has(secretNameOf(row.secret))"
        :disabled="!storageAvailable"
        :aria-label="t('studio.api.variablesDialog.secret.value.aria', { name: row.name })"
        :data-testid="`api-variable-secret-${i}`"
        @update:model-value="(v) => setTyped(i, v)"
      />
      <Input
        v-else
        :model-value="row.value ?? ''"
        :aria-label="t('studio.api.variablesDialog.value.aria')"
        class="font-data"
        :data-testid="`api-variable-value-${i}`"
        @update:model-value="(v) => patch(i, { value: String(v) })"
      />
      <Checkbox
        :model-value="!!row.secret"
        :aria-label="t('common.secret.label')"
        class="size-3.5"
        :data-testid="`api-variable-is-secret-${i}`"
        @update:model-value="(v) => setSecret(i, v === true)"
      >
        <CodiconIcon name="check" :size="12" />
      </Checkbox>
      <Input
        :model-value="row.description ?? ''"
        :aria-label="t('studio.api.variablesDialog.description.aria')"
        :data-testid="`api-variable-description-${i}`"
        @update:model-value="(v) => patch(i, { description: String(v) })"
      />
      <TooltipIconButton
        :icon="ICON.close"
        :label="t('common.action.remove.label')"
        :data-testid="`api-variable-remove-${i}`"
        @click="remove(i)"
      />
    </div>
    <div class="px-1.5">
      <Button variant="toolbar" size="kira" data-testid="api-variable-add" @click="add">
        {{ t('docker.edit.addVariable.label') }}
      </Button>
    </div>
  </div>
</template>
