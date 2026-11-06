<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';

defineProps<{ label: string; description?: string; disabled?: boolean; testid: string }>();
const model = defineModel<boolean>({ required: true });
</script>

<template>
  <Label class="flex flex-row items-center gap-1.5 flex-wrap leading-none">
    <Checkbox
      :model-value="model"
      :disabled="disabled"
      class="size-3.5"
      :data-testid="testid"
      @update:model-value="(v) => (model = v === true)"
    >
      <CodiconIcon name="check" :size="12" />
    </Checkbox>
    <span>{{ label }}</span>
    <span v-if="description" class="w-full text-subtle text-kira-sm leading-normal">{{
      description
    }}</span>
  </Label>
</template>
