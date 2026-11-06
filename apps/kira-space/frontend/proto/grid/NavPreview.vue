<script setup lang="ts">
import { Popover, PopoverAnchor, PopoverContent } from '@theme/components/ui/popover';

export interface NavTarget {
  kind: 'fk' | 'pk';
  /** Viewport rect of the cell the glyph sits in. */
  rect: { left: number; top: number; width: number; height: number };
  column: string;
  value: string;
  /** Canned referenced row, `column: value` lines. */
  lines: string[];
}

defineProps<{ target: NavTarget | null }>();
const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <Popover :open="target !== null" @update:open="(open) => !open && emit('close')">
    <PopoverAnchor as-child>
      <span
        class="pointer-events-none fixed"
        data-testid="fk-preview-anchor"
        :style="{
          left: `${target?.rect.left ?? 0}px`,
          top: `${target?.rect.top ?? 0}px`,
          width: `${target?.rect.width ?? 0}px`,
          height: `${target?.rect.height ?? 0}px`,
        }"
      />
    </PopoverAnchor>
    <PopoverContent align="start" side="bottom" data-testid="fk-preview" @open-auto-focus.prevent>
      <div v-if="target" class="flex flex-col gap-1">
        <div class="text-fg-muted">
          {{ target.kind === 'fk' ? 'References' : 'Referenced by' }} {{ target.column }} =
          {{ target.value }}
        </div>
        <div v-for="line in target.lines" :key="line" class="font-mono">{{ line }}</div>
      </div>
    </PopoverContent>
  </Popover>
</template>
