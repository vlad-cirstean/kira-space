<script setup lang="ts">
import { t } from '@theme/i18n';
import { Handle, Position } from '@vue-flow/core';

// End of the workflow: the target of every result that finishes it.
defineProps<{ data: { w: number; h: number } }>();
</script>

<template>
  <div
    class="flex items-center justify-center rounded-full border border-border bg-bg text-kira-sm text-subtle"
    :style="{ width: `${data.w}px`, height: `${data.h}px` }"
    data-testid="ade-wf-end-node"
  >
    {{ t('space.ade.workflows.route.end.option') }}
    <Handle
      id="in"
      type="target"
      :position="Position.Top"
      class="!size-2 !border-0 !bg-muted-foreground"
    />
  </div>
</template>
