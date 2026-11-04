<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { PopoverContentEmits, PopoverContentProps } from 'reka-ui';
import { PopoverContent, PopoverPortal, useForwardPropsEmits } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

defineOptions({
  inheritAttrs: false,
});

const props = withDefaults(
  defineProps<PopoverContentProps & { class?: HTMLAttributes['class']; destructive?: boolean }>(),
  {
    align: 'center',
    sideOffset: 4,
    destructive: false,
  },
);
const emits = defineEmits<PopoverContentEmits>();

const delegatedProps = reactiveOmit(props, 'class', 'destructive');

const forwarded = useForwardPropsEmits(delegatedProps, emits);

// Same rule as DialogContent: a destructive popover opens with focus on `data-dialog-dismiss`.
function onOpenAutoFocus(event: Event): void {
  if (!props.destructive) return;
  const dismiss = (event.currentTarget as HTMLElement | null)?.querySelector<HTMLElement>(
    '[data-dialog-dismiss]',
  );
  if (!dismiss) return;
  event.preventDefault();
  dismiss.focus();
}
</script>

<template>
  <PopoverPortal>
    <PopoverContent
      data-slot="popover-content"
      v-bind="{ ...$attrs, ...forwarded }"
      @open-auto-focus="onOpenAutoFocus"
      :class="
        cn(
          'bg-elevated text-fg data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-95 data-open:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 ring-border flex flex-col gap-2.5 rounded-kira p-2.5 text-kira-md shadow-kira ring-1 duration-100 z-(--kira-z-popover) w-72 max-w-(--reka-popover-content-available-width) origin-(--reka-popover-content-transform-origin) outline-hidden',
          props.class,
        )
      "
    >
      <slot />
    </PopoverContent>
  </PopoverPortal>
</template>
