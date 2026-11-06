<script setup lang="ts">
import type { ProofDetail } from '@shared/domain/proof';
import { Badge } from '@theme/components/ui/badge';
import { formatDateTime } from '@theme/format';
import { t } from '@theme/i18n';
import TitleBarBase from '@workbench/components/TitleBar.vue';

defineProps<{ detail: ProofDetail; steps: number }>();
</script>

<template>
  <TitleBarBase>
    <div class="flex min-w-0 flex-1 items-center gap-2" data-testid="proof-header">
      <span class="min-w-0 truncate text-kira-md font-medium" data-testid="proof-title">{{
        detail.name
      }}</span>
      <span class="shrink-0 text-kira-sm text-muted-foreground" data-testid="proof-steps">{{
        t('studio.proof.session.steps.label', { count: steps })
      }}</span>
      <span class="shrink-0 text-kira-sm text-muted-foreground">{{
        formatDateTime(detail.closedAt)
      }}</span>
      <Badge v-if="detail.pending" variant="warn" class="shrink-0" data-testid="proof-pending">{{
        t('studio.proof.pending.label')
      }}</Badge>
      <div class="wails-no-drag ml-auto flex shrink-0 items-center gap-1.5">
        <slot />
      </div>
    </div>
  </TitleBarBase>
</template>
