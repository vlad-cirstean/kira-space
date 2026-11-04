<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { SwitchRootEmits, SwitchRootProps } from 'reka-ui';
import { SwitchRoot, SwitchThumb, useForwardPropsEmits } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

// reka-ui emits `data-state="checked"|"unchecked"` (never a bare `data-checked`), so the variants
// below target `data-[state=...]`.
const props = defineProps<SwitchRootProps & { class?: HTMLAttributes['class'] }>();
const emits = defineEmits<SwitchRootEmits>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <SwitchRoot
    data-slot="switch"
    v-bind="forwarded"
    :class="
      cn(
        'peer inline-flex h-3.5 w-6.5 shrink-0 cursor-default items-center rounded-full border border-border bg-field transition-colors disabled:cursor-default disabled:opacity-50 data-[state=checked]:border-primary data-[state=checked]:bg-primary',
        props.class,
      )
    "
  >
    <SwitchThumb
      data-slot="switch-thumb"
      class="pointer-events-none block size-2.5 rounded-full bg-fg transition-transform data-[state=unchecked]:translate-x-0.5 data-[state=checked]:translate-x-3 data-[state=checked]:bg-primary-foreground"
    />
  </SwitchRoot>
</template>
