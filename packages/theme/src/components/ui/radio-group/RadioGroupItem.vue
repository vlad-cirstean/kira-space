<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { RadioGroupItemProps } from 'reka-ui';
import { RadioGroupIndicator, RadioGroupItem, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<RadioGroupItemProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');

const forwardedProps = useForwardProps(delegatedProps);
</script>

<template>
  <RadioGroupItem
    data-slot="radio-group-item"
    v-bind="forwardedProps"
    :class="
      cn(
        'group/radio-group-item peer relative flex aspect-square size-control-inline shrink-0 rounded-full border border-border bg-field transition-colors data-[state=checked]:border-primary data-[state=checked]:bg-primary aria-invalid:border-error aria-invalid:aria-checked:border-primary after:absolute after:-inset-1 disabled:cursor-default disabled:opacity-50',
        props.class,
      )
    "
  >
    <RadioGroupIndicator
      data-slot="radio-group-indicator"
      class="flex size-full items-center justify-center"
    >
      <slot>
        <span class="size-1.5 rounded-full bg-primary-foreground" />
      </slot>
    </RadioGroupIndicator>
  </RadioGroupItem>
</template>
