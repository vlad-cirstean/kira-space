<script setup lang="ts">
import Spinner from '@theme/components/Spinner.vue';
import { Badge } from '@theme/components/ui/badge';
import { computed } from 'vue';
import { opLoadingText, useAdeOpsStore } from '../state/adeOps';

// The status of an op running for a session, for one of a task's branches when `branchId` is set, or
// for the task as a whole.
const props = defineProps<{ taskId?: string; branchId?: string; sessionId?: string }>();
const store = useAdeOpsStore();
const op = computed(() => {
  if (props.sessionId) return store.forSession(props.sessionId)[0];
  if (!props.taskId) return undefined;
  return (
    props.branchId ? store.forBranch(props.taskId, props.branchId) : store.forTask(props.taskId)
  )[0];
});
</script>

<template>
  <Badge v-if="op" variant="warn" data-testid="ade-op-status" :data-op="op.kind">
    <Spinner :size="12" label="" />
    {{ opLoadingText(op.kind) }}
  </Badge>
</template>
