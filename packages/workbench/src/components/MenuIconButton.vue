<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { type MenuItem, useContextMenuStore } from '../state/contextMenu';

// A toolbar icon that opens its items in the shared dropdown, anchored under the button.
// Every other attribute (data-testid, aria-label, pending, class) falls through to the button.
defineOptions({ inheritAttrs: false });
const props = withDefaults(
  defineProps<{ label: string; items: () => MenuItem[]; icon?: string }>(),
  { icon: 'add' },
);
const menu = useContextMenuStore();

function open(e: MouseEvent): void {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  menu.openContextMenuAt(rect.left, rect.bottom + 2, props.items());
}
</script>

<template>
  <TooltipIconButton
    :icon="icon"
    :label="label"
    aria-haspopup="menu"
    v-bind="$attrs"
    @click="open"
  />
</template>
