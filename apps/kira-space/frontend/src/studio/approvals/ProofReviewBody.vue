<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { control } from '../bridge/control';

const props = defineProps<{ item: ApprovalItem }>();

const payload = computed(() => proofReviewPayloadSchema.parse(props.item.payload));
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="proof-review-body">
    <p v-if="payload.reason" class="m-0 whitespace-pre-wrap">{{ payload.reason }}</p>
    <ol class="m-0 flex list-none flex-col gap-2 p-0">
      <li v-for="step in payload.steps" :key="step.ordinal" class="flex flex-col gap-1">
        <span class="text-kira-sm">{{ step.description }}</span>
        <pre
          v-if="step.diff"
          class="m-0 max-h-48 overflow-auto rounded-kira bg-field p-2 font-data text-kira-sm"
        >{{ step.diff }}</pre>
      </li>
    </ol>
    <div>
      <Button
        variant="dialog"
        size="kira-lg"
        data-testid="proof-review-open"
        @click="control.proofOpenWindow(payload.sessionId)"
      >
        {{ t('studio.proof.change.openWindow.label') }}
      </Button>
    </div>
  </div>
</template>
