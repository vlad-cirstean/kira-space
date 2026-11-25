<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { RadioGroupItemProps } from 'reka-ui';
import { RadioGroupItem, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { useId, useSlots } from 'vue';

const props = defineProps<RadioGroupItemProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');

const forwardedProps = useForwardProps(delegatedProps);

const slots = useSlots();
const titleId = useId();
const descriptionId = useId();
</script>

<template>
  <RadioGroupItem
    data-slot="radio-group-card"
    v-bind="forwardedProps"
    :aria-labelledby="titleId"
    :aria-describedby="slots.description ? descriptionId : undefined"
    :class="
      cn(
        'flex w-full min-w-0 flex-col items-start gap-0.5 rounded-kira border border-border bg-bg px-2.5 py-2 text-left text-kira-md text-fg transition-colors',
        'not-data-[disabled]:not-data-[state=checked]:hover:bg-field',
        'data-[state=checked]:border-select-edge data-[state=checked]:bg-select',
        'data-[disabled]:cursor-default data-[disabled]:opacity-50',
        props.class,
      )
    "
  >
    <span :id="titleId" class="font-medium"><slot name="title" /></span>
    <span
      v-if="slots.description"
      :id="descriptionId"
      class="text-kira-sm leading-normal text-muted-foreground"
    >
      <slot name="description" />
    </span>
  </RadioGroupItem>
</template>
