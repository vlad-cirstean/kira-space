<script setup lang="ts">
import Spinner from '@theme/components/Spinner.vue';
import { Badge } from '@theme/components/ui/badge';
import { computed } from 'vue';
import { opLoadingText, useAdeOpsStore } from '../state/adeOps';

// The status of a destructive op running for a task, or for one of its branches when `branchId` is set.
const props = defineProps<{ taskId: string; branchId?: string }>();
const store = useAdeOpsStore();
const op = computed(
  () =>
    (props.branchId
      ? store.forBranch(props.taskId, props.branchId)
      : store.forTask(props.taskId))[0],
);
</script>

<template>
  <Badge v-if="op" variant="warn" data-testid="ade-op-status" :data-op="op.kind">
    <Spinner :size="12" label="" />
    {{ opLoadingText(op.kind) }}
  </Badge>
</template>
