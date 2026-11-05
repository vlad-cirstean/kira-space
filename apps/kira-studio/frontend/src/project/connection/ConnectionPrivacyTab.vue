<script setup lang="ts">
import { FILE_KINDS } from '@shared/domain/connection';
import type { ConnectionFileMask, ConnectionFileModel } from '@shared/domain/connectionConfig';
import type { MaskKind } from '@shared/domain/mask';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { NativeSelect } from '@theme/components/ui/native-select';
import { type MessageKey, t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import { regenerateMaskKey } from '../../state/maskRules';
import CheckboxRow from './CheckboxRow.vue';

// The file's masking rules. They are part of the draft and save with the file; the correlation key
// is separate and regenerates at once.
const props = defineProps<{
  /** The connection's row id; empty while the file was never valid. */
  connectionId: string;
  errorFor: (pointer: string) => string | undefined;
}>();
const file = defineModel<ConnectionFileModel>({ required: true });
const confirmDialogStore = useConfirmDialogStore();
const connectionsStore = useConnectionsStore();

const rules = computed(() => file.value.masks ?? []);
const isFileStyle = computed(() => FILE_KINDS.has(file.value.kind as never));
const secretStatus = computed(() => connectionsStore.secretStorage);

const MASK_KIND_OPTIONS: readonly { value: MaskKind; label: MessageKey }[] = [
  { value: 'name', label: 'common.field.name.label' },
  { value: 'email', label: 'common.field.email.label' },
  { value: 'text', label: 'common.field.text.label' },
  { value: 'number', label: 'common.field.number.label' },
  { value: 'date', label: 'common.field.date.label' },
  { value: 'redact', label: 'studio.project.connectionDialog.redact.label' },
];

const MASK_KIND_EXPLANATION: Record<MaskKind, MessageKey> = {
  name: 'studio.project.connectionDialog.maskKind.name.description',
  email: 'studio.project.connectionDialog.maskKind.email.description',
  text: 'studio.project.connectionDialog.maskKind.text.description',
  number: 'studio.project.connectionDialog.maskKind.number.description',
  date: 'studio.project.connectionDialog.maskKind.date.description',
  redact: 'studio.project.connectionDialog.maskKind.redact.description',
};

// keepHint's label changes per kind; it is hidden for the three kinds where it means nothing.
const KEEP_HINT_LABEL: Partial<Record<string, MessageKey>> = {
  name: 'studio.project.connectionDialog.keep.initials.label',
  email: 'studio.project.connectionDialog.keep.domain.label',
  date: 'studio.project.connectionDialog.keep.year.label',
};

function keepHintLabel(kind: string): string | undefined {
  const key = KEEP_HINT_LABEL[kind];
  return key ? t(key) : undefined;
}

const newTable = ref('');
const newColumn = ref('');
const newKind = ref<MaskKind>('redact');

function setRules(next: ConnectionFileMask[]): void {
  file.value.masks = next;
}

function onAdd(): void {
  const column = newColumn.value.trim();
  if (!column) return;
  const table = newTable.value.trim();
  setRules([
    ...rules.value,
    {
      ...(table && table !== '*' ? { table } : {}),
      column,
      kind: newKind.value,
      ...(newKind.value === 'number' || newKind.value === 'date' ? { correlate: false } : {}),
    },
  ]);
  newTable.value = '';
  newColumn.value = '';
  newKind.value = 'redact';
}

function update(i: number, change: ConnectionFileMask): void {
  setRules(rules.value.map((r, k) => (k === i ? change : r)));
}

function onChangeKind(i: number, kind: string): void {
  const rule = rules.value[i];
  if (!rule) return;
  update(i, { ...rule, kind, ...(kind === 'number' ? { correlate: false } : {}) });
}

// keep_hint and correlate default to true: true is the key left out.
function onToggle(i: number, field: 'keep_hint' | 'correlate'): void {
  const rule = rules.value[i] as ConnectionFileMask;
  const { [field]: current, ...rest } = rule;
  update(i, (current ?? true) ? { ...rest, [field]: false } : rest);
}

function onRemove(i: number): void {
  setRules(rules.value.filter((_, k) => k !== i));
}

const keyError = ref('');
async function onRegenerateKey(): Promise<void> {
  const ok = await confirmDialogStore.confirm({
    title: t('studio.project.connectionDialog.regenerateConfirm.title'),
    message: [
      t('studio.project.connectionDialog.regenerateConfirm.body'),
      t('common.confirm.irreversible.body'),
    ].join(' '),
    confirmLabel: t('studio.common.regenerate.label'),
    tone: 'danger',
  });
  if (!ok) return;
  keyError.value = '';
  try {
    await regenerateMaskKey(props.connectionId);
  } catch (err) {
    keyError.value = errorText(err);
  }
}
</script>

<template>
  <div class="flex flex-col gap-3 p-3" data-testid="connection-section-privacy">
    <FieldDescription class="w-full">
      {{ t('studio.project.connectionDialog.privacy.intro.description') }}
    </FieldDescription>
    <FieldDescription class="w-full" data-testid="connection-privacy-enforce-masking">
      {{ t('studio.project.connectionDialog.mcp.enforceMasking.body') }}
    </FieldDescription>

    <div
      v-if="rules.length"
      class="flex flex-col gap-1 max-h-55 overflow-y-auto"
      data-testid="mask-rule-list"
    >
      <div
        v-for="(rule, i) in rules"
        :key="i"
        class="flex items-center gap-1.5 py-1 border-b border-border"
        data-testid="mask-rule"
      >
        <span class="flex-1 min-w-0 text-fg font-data">
          <TruncatedText :text="`${rule.table || '*'}.${rule.column}`" />
        </span>
        <NativeSelect
          variant="bordered"
          :model-value="rule.kind"
          data-testid="mask-rule-kind"
          @update:model-value="(v) => onChangeKind(i, String(v))"
        >
          <option v-for="o in MASK_KIND_OPTIONS" :key="o.value" :value="o.value">{{
            t(o.label)
          }}</option>
        </NativeSelect>
        <CheckboxRow
          v-if="keepHintLabel(rule.kind)"
          class="flex-none"
          :model-value="rule.keep_hint ?? true"
          :label="keepHintLabel(rule.kind) as string"
          testid="mask-rule-keep-hint"
          @update:model-value="onToggle(i, 'keep_hint')"
        />
        <CheckboxRow
          class="flex-none"
          :model-value="rule.correlate ?? true"
          :disabled="rule.kind === 'number'"
          :label="t('studio.project.connectionDialog.correlate.label')"
          testid="mask-rule-correlate"
          @update:model-value="onToggle(i, 'correlate')"
        />
        <TooltipIconButton
          icon="trash"
          :label="t('studio.project.connectionDialog.removeRule.tooltip')"
          tone="error"
          data-testid="mask-rule-remove"
          @click="onRemove(i)"
        />
      </div>
    </div>
    <FieldDescription v-else class="w-full">{{
      t('studio.project.connectionDialog.privacy.noRules.emptyHint')
    }}</FieldDescription>
    <FieldError v-if="props.errorFor('/masks')" data-testid="mask-rule-error">{{
      props.errorFor('/masks')
    }}</FieldError>

    <div class="flex gap-2 items-center">
      <Input
        v-model="newTable"
        placeholder="*"
        class="font-data"
        data-testid="mask-rule-add-table"
      />
      <Input
        v-model="newColumn"
        :placeholder="t('studio.project.connectionDialog.column.placeholder')"
        class="font-data"
        data-testid="mask-rule-add-column"
      />
      <NativeSelect v-model="newKind" variant="bordered" data-testid="mask-rule-add-kind">
        <option v-for="o in MASK_KIND_OPTIONS" :key="o.value" :value="o.value">{{
          t(o.label)
        }}</option>
      </NativeSelect>
      <Button variant="dialog" size="kira-lg" data-testid="mask-rule-add" @click="onAdd">{{
        t('common.action.add.label')
      }}</Button>
    </div>
    <FieldDescription class="w-full">{{ t(MASK_KIND_EXPLANATION[newKind]) }}</FieldDescription>

    <FieldDescription v-if="!connectionId" class="w-full">{{
      t('studio.project.connectionDialog.privacy.saveFirst.hint')
    }}</FieldDescription>
    <Button
      v-else
      variant="dialog"
      size="kira-lg"
      class="self-start"
      data-testid="mask-regenerate-key"
      @click="onRegenerateKey"
    >
      {{ t('studio.project.connectionDialog.regenerate.label') }}
    </Button>
    <Alert v-if="keyError" variant="error" data-testid="mask-key-error">
      <AlertDescription>{{ keyError }}</AlertDescription>
    </Alert>
    <Alert
      v-if="!isFileStyle && secretStatus?.insecureFallback"
      data-testid="mask-key-credential-note"
      variant="warn"
    >
      <AlertDescription>
        {{ t('studio.project.connectionDialog.privacy.keyFallback.body') }}
      </AlertDescription>
    </Alert>
  </div>
</template>
