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
        'peer relative flex size-control-inline shrink-0 items-center justify-center rounded-kira-sm border border-toggle-off-edge bg-field transition-colors focus-visible:outline-offset-2 aria-checked:border-toggle-on-edge aria-checked:bg-toggle-on aria-checked:text-toggle-mark aria-[checked=mixed]:border-toggle-on-edge aria-[checked=mixed]:bg-toggle-on aria-[checked=mixed]:text-toggle-mark aria-invalid:border-error aria-invalid:aria-checked:border-toggle-on-edge group-has-disabled/field:opacity-50 after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-default disabled:opacity-50',
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
