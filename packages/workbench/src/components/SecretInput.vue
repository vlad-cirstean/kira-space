<script setup lang="ts">
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// Write-only secret entry. It never receives a stored value: `set` only says whether one exists,
// and shows in the placeholder. Attributes (testid, aria-label, class) fall through to the input.
const props = defineProps<{ id?: string; set?: boolean; disabled?: boolean }>();
const value = defineModel<string>({ required: true });

const placeholder = computed(() => {
  if (props.set === undefined) return undefined;
  return props.set ? t('common.secret.saved.placeholder') : t('docker.edit.unset.status');
});
</script>

<template>
  <Input
    :id="id"
    :model-value="value"
    type="password"
    autocomplete="new-password"
    class="font-data"
    :disabled="disabled"
    :placeholder="placeholder"
    @update:model-value="(v) => (value = String(v))"
  />
</template>
