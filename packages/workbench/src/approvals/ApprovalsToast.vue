<script setup lang="ts">
import type { ApprovalItem } from '@shared/domain/approvals';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { floatingSurface } from '@theme/lib/frames';
import { useApprovalsStore } from '@workbench/state/approvals';
import { computed } from 'vue';
import { formatClock, useCountdown } from './useCountdown';

const props = defineProps<{ items: ApprovalItem[] }>();

const store = useApprovalsStore();
const soonest = computed(() => {
  const times = props.items.map((i) => i.expiresAt).filter((e) => e > 0);
  return times.length ? Math.min(...times) : null;
});
const remaining = useCountdown(() => soonest.value);
</script>

<template>
  <div
    role="status"
    aria-live="polite"
    :class="[
      floatingSurface,
      'fixed top-[calc(var(--kira-titlebar-h)+8px)] right-4 z-(--kira-z-dialog) flex items-center gap-3 p-3',
    ]"
    data-testid="approvals-toast"
  >
    <div class="flex flex-col">
      <span data-testid="approvals-toast-count">
        {{ t('workbench.approvals.toast.count.status', { count: items.length }) }}
      </span>
      <span
        v-if="remaining !== null"
        class="text-kira-sm text-muted-foreground"
        data-testid="approvals-toast-expires"
      >
        {{ t('workbench.approvals.toast.expires.status', { time: formatClock(remaining) }) }}
      </span>
    </div>
    <Button
      variant="dialog-primary"
      size="kira-lg"
      data-testid="approvals-toast-review"
      @click="store.open()"
    >
      {{ t('common.action.show.label') }}
    </Button>
  </div>
</template>
