<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { reviewOptionPayloadSchema } from '@shared/domain/dbmcp';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';

// "Review the result before the AI gets it" on a pre-run item. Already on by the connection's or
// environment's settings: shown checked and disabled, the reason in a tooltip. Otherwise a tick rides the
// approval as an option.
const props = defineProps<{ item: ApprovalItem }>();
const emit = defineEmits<{ options: [value: { reviewResult: boolean }] }>();

// db.query, db.exemption and api.request carry the same two review fields; the db kinds add a mode.
const payload = computed(() => reviewOptionPayloadSchema.parse(props.item.payload));
const isApi = computed(() => props.item.kind === 'api.request');
const ticked = ref(false);
function onTick(v: boolean): void {
  ticked.value = v;
  emit('options', { reviewResult: v });
}
</script>

<template>
  <Tooltip v-if="payload.reviewOffered" :disabled="!payload.reviewResult">
    <TooltipTrigger as-child>
      <Label class="flex shrink-0 items-center gap-1.5 font-normal">
        <Checkbox
          :model-value="payload.reviewResult || ticked"
          :disabled="payload.reviewResult"
          :data-testid="isApi ? 'api-approval-review-result' : 'db-mcp-approval-review-result'"
          @update:model-value="(v) => onTick(v === true)"
        />
        {{ t('studio.dbMcp.approval.reviewResultShort.label') }}
      </Label>
    </TooltipTrigger>
    <TooltipContent>{{
      isApi
        ? t('studio.dbMcp.approval.api.reviewResultOn.body')
        : t('studio.dbMcp.approval.reviewResultOn.body', { mode: payload.mode })
    }}</TooltipContent>
  </Tooltip>
</template>
