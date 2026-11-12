<script setup lang="ts">
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';

// Save and Discard for the open workflow editor; both are off while nothing is edited.
const props = defineProps<{ dirty: boolean; saving: boolean }>();
const actionable = computed(() => props.dirty && !props.saving);
const emit = defineEmits<{ save: []; discard: [] }>();
</script>

<template>
  <div class="flex flex-wrap items-center gap-2" data-testid="ade-wf-savebar">
    <span class="min-w-0 flex-1 text-kira-sm text-subtle">{{
      t('space.ade.workflows.saveBar.applies.hint')
    }}</span>
    <Button
      variant="dialog-danger"
      size="kira-lg"
      :disabled="!actionable"
      data-testid="ade-wf-discard"
      @click="emit('discard')"
    >
      {{ t('common.action.discard.label') }}
    </Button>
    <Button
      variant="dialog-primary"
      size="kira-lg"
      :disabled="!dirty"
      :pending="saving"
      data-testid="ade-wf-save"
      @click="emit('save')"
    >
      {{ t('common.action.save.label') }}
    </Button>
  </div>
</template>
