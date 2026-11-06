<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';

// Per-row icon button without its own Tooltip: 2 reka Tooltip instances per row cost ~51 ms per
// fast-flick frame (P161). DocumentView.vue owns the one shared tooltip and reads `data-tip` off
// this wrapper through delegated pointer/focus listeners.
defineOptions({ inheritAttrs: false });

defineProps<{ icon: string; label: string; disabled?: boolean }>();
</script>

<template>
  <!-- Focusable wrapper, same shape as TooltipDisabledTrigger: Blink dispatches no pointer event on
       a disabled control, so the tip needs a live ancestor to hang off. -->
  <!-- Only a disabled button needs the wrapper as its tab stop; an enabled one is its own. -->
  <span :tabindex="disabled ? 0 : undefined" class="inline-flex" :data-tip="label">
    <Button
      variant="toolbar"
      size="kira-icon"
      :aria-label="label"
      :disabled="disabled"
      v-bind="$attrs"
    >
      <CodiconIcon :name="icon" :size="13" />
    </Button>
  </span>
</template>
