<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field } from '@theme/components/ui/field';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { useId } from 'vue';
import type { SettingsPaneProps } from './types';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Data'"` branch.
const props = defineProps<SettingsPaneProps>();

const PAGE_SIZES = [10, 100, 1000, 10000] as const;

function onDefaultPageSizeChange(rawValue: unknown): void {
  const value = Number(rawValue);
  const pageSize = PAGE_SIZES.find((size) => size === value);
  if (!pageSize) return;
  props.draft.data.defaultPageSize = pageSize;
}

// P110 I2-26: `for`/`id` preserves the old label-wraps-control implicit association (see
// FontSizeField.vue's own precedent comment) now that the field wrapper is a plain <Field> div.
const defaultPageSizeId = useId();
</script>

<template>
  <div class="contents" v-show="active">
    <Field>
      <div class="flex items-center justify-between gap-1">
        <Label :for="defaultPageSizeId">{{ t('studio.settings.data.pageSize.label') }}</Label>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-data-defaultPageSize"
          :disabled-trigger="isAtDefault('data', 'defaultPageSize')"
          :disabled="isAtDefault('data', 'defaultPageSize')"
          @click="resetLeaf('data', 'defaultPageSize')"
        />
      </div>
      <NativeSelect
        :id="defaultPageSizeId"
        variant="bordered"
        size="kira-lg"
        class="self-start"
        data-testid="settings-default-page-size"
        :model-value="draft.data.defaultPageSize"
        @update:model-value="onDefaultPageSizeChange"
      >
        <option v-for="size in PAGE_SIZES" :key="size" :value="size">{{ size }}</option>
      </NativeSelect>
    </Field>
  </div>
</template>
