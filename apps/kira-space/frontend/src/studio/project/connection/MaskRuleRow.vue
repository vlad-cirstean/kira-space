<script setup lang="ts">
import type { ConnectionFileMask } from '@shared/domain/connectionConfig';
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import CheckboxRow from './CheckboxRow.vue';
import { KEEP_HINT_LABEL, MASK_KIND_OPTIONS, maskExample } from './maskKinds';

// One masking rule: the column name it matches, its kind, its options and what the agent sees.
const rule = defineModel<ConnectionFileMask>({ required: true });
const emit = defineEmits<{ remove: [] }>();

const keepHintLabel = computed(() => {
  const key = KEEP_HINT_LABEL[rule.value.kind];
  return key ? t(key) : undefined;
});
const tableIgnored = computed(() => !!rule.value.table && rule.value.table !== '*');
const example = computed(() => maskExample(rule.value));
const isNumber = computed(() => rule.value.kind === 'number');

function onChangeKind(kind: string): void {
  rule.value = { ...rule.value, kind, ...(kind === 'number' ? { correlate: false } : {}) };
}

// keep_hint and correlate default to true: true is the key left out.
function onToggle(field: 'keep_hint' | 'correlate'): void {
  const { [field]: current, ...rest } = rule.value;
  rule.value = (current ?? true) ? { ...rest, [field]: false } : rest;
}
</script>

<template>
  <div class="flex flex-col gap-1 py-1.5 border-b border-border" data-testid="mask-rule">
    <div class="flex items-center gap-1.5">
      <span class="min-w-0 flex-1 text-fg font-data">
        <TruncatedText :text="rule.column" />
      </span>
      <Tooltip v-if="tableIgnored">
        <TooltipTrigger as-child>
          <Badge variant="warn" data-testid="mask-rule-table-ignored">{{
            t('studio.project.connectionDialog.privacy.tableIgnored.status')
          }}</Badge>
        </TooltipTrigger>
        <TooltipContent>{{
          t('studio.project.connectionDialog.privacy.tableIgnored.tooltip', {
            table: rule.table ?? '',
          })
        }}</TooltipContent>
      </Tooltip>
      <NativeSelect
        variant="bordered"
        :model-value="rule.kind"
        :aria-label="t('studio.project.connectionDialog.privacy.maskAs.label')"
        data-testid="mask-rule-kind"
        @update:model-value="(v) => onChangeKind(String(v))"
      >
        <option v-for="o in MASK_KIND_OPTIONS" :key="o.value" :value="o.value">{{
          t(o.label)
        }}</option>
      </NativeSelect>
      <TooltipIconButton
        icon="trash"
        :label="t('studio.project.connectionDialog.removeRule.tooltip')"
        tone="error"
        data-testid="mask-rule-remove"
        @click="emit('remove')"
      />
    </div>
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-kira-sm">
      <CheckboxRow
        v-if="keepHintLabel"
        class="flex-none"
        :model-value="rule.keep_hint ?? true"
        :label="keepHintLabel"
        testid="mask-rule-keep-hint"
        @update:model-value="onToggle('keep_hint')"
      />
      <Tooltip>
        <TooltipTrigger as-child>
          <CheckboxRow
            class="flex-none"
            :model-value="!isNumber && (rule.correlate ?? true)"
            :disabled="isNumber"
            :label="t('studio.project.connectionDialog.privacy.tag.label')"
            testid="mask-rule-correlate"
            @update:model-value="onToggle('correlate')"
          />
        </TooltipTrigger>
        <TooltipContent>{{
          t(
            isNumber
              ? 'studio.project.connectionDialog.privacy.tagNumber.tooltip'
              : 'studio.project.connectionDialog.privacy.tag.tooltip',
          )
        }}</TooltipContent>
      </Tooltip>
      <span class="flex min-w-0 items-center gap-1.5 text-subtle">
        <span class="font-data">{{ example.input }}</span>
        <CodiconIcon name="arrow-right" :size="12" />
        <span>{{ t('studio.project.connectionDialog.privacy.agentSees.label') }}</span>
        <span class="font-data text-fg" data-testid="mask-rule-example">{{ example.output }}</span>
      </span>
    </div>
  </div>
</template>
