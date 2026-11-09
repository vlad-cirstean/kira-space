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
  <Button
    variant="link"
    size="inline"
    class="text-kira-sm font-normal text-muted-foreground underline decoration-border underline-offset-3"
    data-testid="proof-review-open"
    @click="control.proofOpenWindow(payload.sessionId)"
  >
    {{ t('studio.proof.change.openWindow.label') }}
  </Button>
</template>
