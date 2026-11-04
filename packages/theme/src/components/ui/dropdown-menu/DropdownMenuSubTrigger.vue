<script setup lang="ts">
import { ChevronRightIcon } from '@lucide/vue';
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { DropdownMenuSubTriggerProps } from 'reka-ui';
import { DropdownMenuSubTrigger, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<
  DropdownMenuSubTriggerProps & { class?: HTMLAttributes['class']; inset?: boolean }
>();

const delegatedProps = reactiveOmit(props, 'class', 'inset');
const forwardedProps = useForwardProps(delegatedProps);
</script>

<template>
  <DropdownMenuSubTrigger
    data-slot="dropdown-menu-sub-trigger"
    :data-inset="inset ? '' : undefined"
    v-bind="forwardedProps"
    :class="
      cn(
        'focus:bg-field focus:text-fg data-open:bg-field data-open:text-fg not-data-[variant=danger]:focus:**:text-fg h-control gap-1.5 rounded-kira-sm px-1.5 text-kira-md data-inset:pl-7 [&_svg:not([class*=size-])]:size-4 flex cursor-default items-center outline-hidden select-none [&_svg]:pointer-events-none [&_svg]:shrink-0',
        props.class,
      )
    "
  >
    <slot />
    <ChevronRightIcon class="cn-rtl-flip ml-auto" />
  </DropdownMenuSubTrigger>
</template>
