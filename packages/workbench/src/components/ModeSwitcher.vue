<script setup lang="ts" generic="M extends string">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { tabChipVariants } from '@theme/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { cn } from '@theme/lib/utils';
import type { ModeRegistry } from '../modes';

defineProps<{
  order: readonly M[];
  modes: ModeRegistry<M>;
  active: M;
}>();

const emit = defineEmits<{ select: [mode: M] }>();
</script>

<template>
  <!-- Centered on the bar's true full width via absolute positioning, not `justify-content: center`
       in the flex row, which would center within the padded box and read off-centre. -->
  <div
    class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 flex items-center gap-0.5"
  >
    <!-- Icon-only tabs; the label is the aria-label and the tooltip. wails-no-drag: isDraggableEvent
         (drag.ts) reads the target's computed style, so every interactive child of the wails-drag
         TitleBar root must override it, or a tab click would also start a window drag. -->
    <Tooltip v-for="mode in order" :key="mode">
      <TooltipTrigger as-child>
        <button
          type="button"
          class="wails-no-drag"
          :class="
            cn(
              tabChipVariants({ active: active === mode, size: 'icon' }),
              'w-control-lg justify-center',
            )
          "
          :aria-label="modes[mode].label"
          :aria-pressed="active === mode"
          data-testid="mode-tab"
          :data-mode="mode"
          @click="emit('select', mode)"
        >
          <!-- Icon at its own 16px design size, so the glyph fills its box (no per-glyph advance slack). -->
          <span
            class="size-4 flex items-center justify-center shrink-0 leading-3.5"
            data-testid="mode-tab-icon"
            ><CodiconIcon :name="modes[mode].icon" :size="16" /></span
          >
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">{{ modes[mode].label }}</TooltipContent>
    </Tooltip>
  </div>
</template>
