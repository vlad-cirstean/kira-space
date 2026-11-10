<script setup lang="ts">
import type { ConnectionFileMask } from '@shared/domain/connectionConfig';
import type { MaskKind } from '@shared/domain/mask';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldError } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';
import MaskRuleRow from './MaskRuleRow.vue';
import {
  defaultCorrelateFor,
  MASK_KIND_EXPLANATION,
  MASK_KIND_OPTIONS,
  maskExample,
} from './maskKinds';

// The rules and the add row. The rules are part of the connection draft.
const props = defineProps<{
  errorFor: (pointer: string) => string | undefined;
  /** True when a rule already masks this column name, in any table. */
  hasRule: (column: string) => boolean;
}>();
const emit = defineEmits<{ add: [column: string, kind: MaskKind] }>();
const masks = defineModel<ConnectionFileMask[]>({ required: true });

const newColumn = ref('');
const newKind = ref<MaskKind>('redact');
const duplicate = ref('');
const newExample = computed(() =>
  maskExample({ kind: newKind.value, correlate: defaultCorrelateFor(newKind.value) }),
);

function onAdd(): void {
  const column = newColumn.value.trim();
  if (!column) return;
  if (props.hasRule(column)) {
    duplicate.value = column;
    return;
  }
  emit('add', column, newKind.value);
  newColumn.value = '';
  newKind.value = 'redact';
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div
      v-if="masks.length"
      class="flex flex-col max-h-60 overflow-y-auto"
      data-testid="mask-rule-list"
    >
      <MaskRuleRow
        v-for="(rule, i) in masks"
        :key="i"
        :model-value="rule"
        @update:model-value="(v) => (masks = masks.map((r, k) => (k === i ? v : r)))"
        @remove="masks = masks.filter((_, k) => k !== i)"
      />
    </div>
    <EmptyState
      v-else
      icon="shield"
      :title="t('studio.project.connectionDialog.privacy.rulesEmpty.empty')"
      :description="
        t('studio.project.connectionDialog.privacy.rulesEmpty.emptyHint', {
          item: t('studio.grid.menu.markSensitive.label'),
        })
      "
      data-testid="mask-rule-empty"
    />
    <FieldError v-if="props.errorFor('/masks')" data-testid="mask-rule-error">{{
      props.errorFor('/masks')
    }}</FieldError>

    <div class="flex gap-2 items-center">
      <Input
        v-model="newColumn"
        :placeholder="t('studio.project.connectionDialog.column.placeholder')"
        class="font-data"
        data-testid="mask-rule-add-column"
        @update:model-value="duplicate = ''"
        @keydown.enter.prevent="onAdd"
      />
      <NativeSelect
        v-model="newKind"
        variant="bordered"
        :aria-label="t('studio.project.connectionDialog.privacy.maskAs.label')"
        data-testid="mask-rule-add-kind"
      >
        <option v-for="o in MASK_KIND_OPTIONS" :key="o.value" :value="o.value">{{
          t(o.label)
        }}</option>
      </NativeSelect>
      <Button variant="dialog" size="kira-lg" data-testid="mask-rule-add" @click="onAdd">{{
        t('common.action.add.label')
      }}</Button>
    </div>
    <FieldError v-if="duplicate" data-testid="mask-rule-duplicate">{{
      t('studio.project.connectionDialog.privacy.rulesDuplicate.error', { column: duplicate })
    }}</FieldError>
    <FieldDescription class="w-full">
      {{ t(MASK_KIND_EXPLANATION[newKind]) }}
      <span class="inline-flex items-center gap-1 font-data">
        {{ newExample.input }}
        <CodiconIcon name="arrow-right" :size="12" />
        <span data-testid="mask-rule-add-example">{{ newExample.output }}</span>
      </span>
    </FieldDescription>
  </div>
</template>
