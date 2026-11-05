<script setup lang="ts">
import { t } from '@theme/i18n';
import { ref } from 'vue';
import CheckboxRow from '../project/connection/CheckboxRow.vue';

// "Review the result before the AI gets it" on a pre-run item. Already on by the connection's
// settings: shown checked and disabled. Otherwise a tick rides the approval as an option.
const props = defineProps<{ mode: string; reviewed: boolean }>();
const emit = defineEmits<{ options: [value: { reviewResult: boolean }] }>();

const ticked = ref(false);
function onTick(v: boolean): void {
  ticked.value = v;
  emit('options', { reviewResult: v });
}
</script>

<template>
  <CheckboxRow
    :model-value="props.reviewed || ticked"
    :disabled="props.reviewed"
    :label="t('studio.dbMcp.approval.reviewResult.label')"
    :description="
      props.reviewed
        ? t('studio.dbMcp.approval.reviewResultOn.body', { mode: props.mode })
        : undefined
    "
    testid="db-mcp-approval-review-result"
    @update:model-value="onTick"
  />
</template>
