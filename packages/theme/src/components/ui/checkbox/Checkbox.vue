<script setup lang="ts">
import { CheckIcon, MinusIcon } from '@lucide/vue';
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { CheckboxRootEmits, CheckboxRootProps } from 'reka-ui';
import { CheckboxIndicator, CheckboxRoot, useForwardPropsEmits } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<CheckboxRootProps & { class?: HTMLAttributes['class'] }>();
const emits = defineEmits<CheckboxRootEmits>();

const delegatedProps = reactiveOmit(props, 'class');

const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <CheckboxRoot
    v-slot="slotProps"
    data-slot="checkbox"
    v-bind="forwarded"
    :class="
      cn(
        'peer relative flex size-control-inline shrink-0 items-center justify-center rounded-kira-sm border border-control-edge bg-field transition-colors aria-checked:border-on aria-checked:bg-on aria-checked:text-on-fg aria-[checked=mixed]:border-on aria-[checked=mixed]:bg-on aria-[checked=mixed]:text-on-fg aria-invalid:border-error aria-invalid:aria-checked:border-on group-has-disabled/field:opacity-50 after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-default disabled:opacity-50',
        props.class,
      )
    "
  >
    <CheckboxIndicator
      data-slot="checkbox-indicator"
      class="[&>svg]:size-2.5 grid place-content-center text-current transition-none"
    >
      <slot v-bind="slotProps">
        <MinusIcon v-if="slotProps.state === 'indeterminate'" />
        <CheckIcon v-else />
      </slot>
    </CheckboxIndicator>
  </CheckboxRoot>
</template>
