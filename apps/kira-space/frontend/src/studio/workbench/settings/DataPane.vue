<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import type { SettingsPaneProps } from '../../../workbench/settings/types';

// P103 Part 2 (§5.5): extracted verbatim from workbench/SettingsDialog.vue's own
// `v-else-if="activeSection === 'Data'"` branch.
const props = defineProps<SettingsPaneProps>();

const PAGE_SIZES = [10, 100, 1000, 10000] as const;

const PAGE_SIZE_ITEMS = computed(() =>
  PAGE_SIZES.map((size) => ({
    value: String(size),
    label: String(size),
    testid: `settings-default-page-size-${size}`,
  })),
);

function onDefaultPageSizeChange(rawValue: unknown): void {
  const value = Number(rawValue);
  const pageSize = PAGE_SIZES.find((size) => size === value);
  if (!pageSize) return;
  props.draft.data.defaultPageSize = pageSize;
}
</script>

<template>
  <div class="contents" v-show="active">
    <Field>
      <div class="flex items-center justify-between gap-1">
        <FieldLabel>{{ t('studio.settings.data.pageSize.label') }}</FieldLabel>
        <TooltipIconButton
          icon="discard"
          :label="t('common.action.resetToDefault.label')"
          data-testid="settings-reset-data-defaultPageSize"
          :disabled-trigger="isAtDefault('data', 'defaultPageSize')"
          :disabled="isAtDefault('data', 'defaultPageSize')"
          @click="resetLeaf('data', 'defaultPageSize')"
        />
      </div>
      <SecondaryTabs
        variant="choice"
        size="kira-lg"
        class="self-start"
        :aria-label="t('studio.settings.data.pageSize.label')"
        :model-value="String(draft.data.defaultPageSize)"
        :items="PAGE_SIZE_ITEMS"
        data-testid="settings-default-page-size"
        @update:model-value="onDefaultPageSizeChange"
      />
    </Field>
  </div>
</template>
