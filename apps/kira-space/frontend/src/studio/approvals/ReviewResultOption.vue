<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { dbExemptionPayloadSchema, dbQueryPayloadSchema } from '@shared/domain/dbmcp';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Checkbox } from '@theme/components/ui/checkbox';
import { Label } from '@theme/components/ui/label';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { computed, ref } from 'vue';

// "Review the result before the AI gets it" on a pre-run item. Already on by the connection's
// settings: shown checked and disabled, the reason in a tooltip. Otherwise a tick rides the
// approval as an option.
const props = defineProps<{ item: ApprovalItem }>();
const emit = defineEmits<{ options: [value: { reviewResult: boolean }] }>();

// db.query and db.exemption carry the same review fields.
const payload = computed(() =>
  (props.item.kind === 'db.exemption' ? dbExemptionPayloadSchema : dbQueryPayloadSchema).parse(
    props.item.payload,
  ),
);
const ticked = ref(false);
function onTick(v: boolean): void {
  ticked.value = v;
  emit('options', { reviewResult: v });
}
</script>

<template>
  <Tooltip v-if="payload.reviewOffered" :disabled="!payload.reviewResult">
    <TooltipTrigger as-child>
      <Label class="flex shrink-0 items-center gap-1.5 text-kira-md font-normal">
        <Checkbox
          :model-value="payload.reviewResult || ticked"
          :disabled="payload.reviewResult"
          class="size-3.5"
          data-testid="db-mcp-approval-review-result"
          @update:model-value="(v) => onTick(v === true)"
        >
          <CodiconIcon name="check" :size="12" />
        </Checkbox>
        {{ t('studio.dbMcp.approval.reviewResultShort.label') }}
      </Label>
    </TooltipTrigger>
    <TooltipContent>{{
      t('studio.dbMcp.approval.reviewResultOn.body', { mode: payload.mode })
    }}</TooltipContent>
  </Tooltip>
</template>
