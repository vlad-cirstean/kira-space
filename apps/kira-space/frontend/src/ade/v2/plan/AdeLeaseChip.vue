<script setup lang="ts">
import { t } from '@theme/i18n';
import { computed } from 'vue';
import AdeChip from '../AdeChip.vue';
import AdeTip from '../AdeTip.vue';
import { useTaskLeases } from '../leases/useTaskLeases';

// A task's lease state on its row: the queue it holds, else its place in line.
const props = defineProps<{ taskId: string }>();
const { byTask } = useTaskLeases();
const lease = computed(() => byTask.value.get(props.taskId));
const holds = computed(() => lease.value?.position === 0);
const args = computed(() => ({
  queue: lease.value?.queue ?? '',
  position: lease.value?.position ?? 0,
}));
</script>

<template>
  <AdeTip
    v-if="lease"
    :text="
      holds
        ? t('space.ade.leases.chip.holds.tooltip', args)
        : t('space.ade.leases.chip.waits.tooltip', args)
    "
  >
    <span class="inline-flex shrink-0" data-testid="ade-lease-chip" :data-holds="holds">
      <AdeChip
        :label="
          holds
            ? t('space.ade.leases.chip.holds.label', args)
            : t('space.ade.leases.chip.waits.label', args)
        "
        :tone="holds ? 'blue' : 'grey'"
        class="max-w-40 truncate"
      />
    </span>
  </AdeTip>
</template>
