<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { formatElapsed } from '@theme/format';
import { t } from '@theme/i18n';
import PendingToast from '@workbench/components/PendingToast.vue';
import { computed } from 'vue';
import { useCountdown } from './useCountdown';

const props = defineProps<{ items: ApprovalItem[]; deferred?: boolean }>();
const emit = defineEmits<{ show: [] }>();

const soonest = computed(() => {
  const times = props.items.map((i) => i.expiresAt).filter((e) => e > 0);
  return times.length ? Math.min(...times) : null;
});
const remaining = useCountdown(() => soonest.value);
</script>

<template>
  <PendingToast testid="approvals-toast" show-testid="approvals-toast-review" @show="emit('show')">
    <span data-testid="approvals-toast-count">
      {{ t('workbench.approvals.toast.count.status', { count: items.length }) }}
    </span>
    <span
      v-if="deferred"
      class="text-kira-sm text-muted-foreground"
      data-testid="approvals-toast-typing"
    >
      {{ t('workbench.prompts.toast.typing.status') }}
    </span>
    <span
      v-if="remaining !== null"
      class="text-kira-sm text-muted-foreground"
      data-testid="approvals-toast-expires"
    >
      {{ t('workbench.approvals.toast.expires.status', { time: formatElapsed(remaining * 1000) }) }}
    </span>
  </PendingToast>
</template>
