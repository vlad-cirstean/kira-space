<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { DropdownMenuItemProps } from 'reka-ui';
import { DropdownMenuItem, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = withDefaults(
  defineProps<
    DropdownMenuItemProps & {
      class?: HTMLAttributes['class'];
      inset?: boolean;
      variant?: 'default' | 'danger';
    }
  >(),
  {
    variant: 'default',
  },
);

const delegatedProps = reactiveOmit(props, 'inset', 'variant', 'class');

const forwardedProps = useForwardProps(delegatedProps);
</script>

<template>
  <DropdownMenuItem
    data-slot="dropdown-menu-item"
    :data-inset="inset ? '' : undefined"
    :data-variant="variant"
    v-bind="forwardedProps"
    :class="
      cn(
        'group/dropdown-menu-item relative flex h-control cursor-default items-center gap-1.5 rounded-kira-sm px-1.5 text-kira-md outline-hidden select-none focus:bg-field focus:text-fg not-data-[variant=danger]:focus:**:text-fg data-[variant=danger]:text-error data-[variant=danger]:focus:bg-error/15 data-[variant=danger]:focus:text-error data-[variant=danger]:*:[svg]:text-error data-inset:pl-7 data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg:not([class*=size-])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0',
        props.class,
      )
    "
  >
    <slot />
  </DropdownMenuItem>
</template>
