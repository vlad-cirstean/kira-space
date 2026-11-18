<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// Form save bar: outline Discard, primary Save, `#start` for a hint, size or helper action.
const props = withDefaults(
  defineProps<{
    dirty: boolean;
    saving: boolean;
    /** Save enabled; defaults to `dirty`. */
    canSave?: boolean;
    testidPrefix: string;
    saveLabel?: string;
  }>(),
  { canSave: undefined, saveLabel: undefined },
);
const emit = defineEmits<{ save: []; discard: [] }>();
const discardOff = computed(() => !props.dirty || props.saving);
</script>

<template>
  <div class="flex flex-wrap items-center gap-2" :data-testid="`${props.testidPrefix}-savebar`">
    <div class="mr-auto flex min-w-0 flex-1 items-center gap-2 text-kira-sm text-subtle">
      <slot name="start" />
    </div>
    <!-- conventions-allow N17: second footer action; one emphasised button rightmost (RD.2) -->
    <Button
      variant="dialog"
      size="kira-lg"
      :disabled="discardOff"
      :data-testid="`${props.testidPrefix}-discard`"
      @click="emit('discard')"
    >
      {{ t('common.action.discard.label') }}
    </Button>
    <Button
      variant="dialog-primary"
      size="kira-lg"
      :disabled="!(props.canSave ?? dirty)"
      :pending="saving"
      :data-testid="`${props.testidPrefix}-save`"
      @click="emit('save')"
    >
      {{ props.saveLabel ?? t('common.action.save.label') }}
    </Button>
  </div>
</template>
