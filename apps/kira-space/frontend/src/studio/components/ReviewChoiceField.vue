<script setup lang="ts">
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import { Field, FieldLabel } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// One response-review setting: inherit follows the app setting (its label shows the current value),
// review always holds, send never holds. Used by the connection and the API environment.
const props = defineProps<{
  label: string;
  /** The app setting inherit resolves to. */
  appOn: boolean;
  disabled?: boolean;
  testid: string;
}>();
const model = defineModel<string>({ required: true });

const items = computed(() =>
  [
    {
      value: 'inherit',
      label: t(
        props.appOn
          ? 'studio.project.connectionDialog.mcp.reviewInheritOn.option'
          : 'studio.project.connectionDialog.mcp.reviewInheritOff.option',
      ),
    },
    { value: 'review', label: t('studio.project.connectionDialog.mcp.reviewAlways.option') },
    { value: 'send', label: t('studio.project.connectionDialog.mcp.reviewNever.option') },
  ].map((i) => ({ ...i, testid: `${props.testid}-${i.value}`, disabled: props.disabled })),
);
</script>

<template>
  <Field>
    <FieldLabel>{{ label }}</FieldLabel>
    <SecondaryTabs
      v-model="model"
      variant="choice"
      size="kira-lg"
      :aria-label="label"
      :items="items"
      :data-testid="testid"
    />
  </Field>
</template>
