<script setup lang="ts">
import { CheckIcon } from '@lucide/vue';
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
        'peer relative flex size-control-inline shrink-0 items-center justify-center rounded-kira-xs border border-border bg-field transition-colors data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground data-[state=indeterminate]:border-primary data-[state=indeterminate]:bg-primary data-[state=indeterminate]:text-primary-foreground aria-invalid:border-error aria-invalid:aria-checked:border-primary group-has-disabled/field:opacity-50 after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-default disabled:opacity-50',
        props.class,
      )
    "
  >
    <CheckboxIndicator
      data-slot="checkbox-indicator"
      class="[&>svg]:size-2.5 grid place-content-center text-current transition-none"
    >
      <slot v-bind="slotProps">
        <CheckIcon />
      </slot>
    </CheckboxIndicator>
  </CheckboxRoot>
</template>
