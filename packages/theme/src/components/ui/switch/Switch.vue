<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { SwitchRootEmits, SwitchRootProps } from 'reka-ui';
import { SwitchRoot, SwitchThumb, useForwardPropsEmits } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

// Root variants key on `aria-checked`: an `as-child` tooltip trigger overwrites the root's
// `data-state` with its own open/closed state. The thumb owns its `data-state`, so it keeps it.
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
        'peer inline-flex h-3.5 w-6.5 shrink-0 cursor-default items-center rounded-full border border-toggle-off-edge bg-field transition-colors focus-visible:outline-offset-2 disabled:cursor-default disabled:opacity-50 aria-checked:border-toggle-on-edge aria-checked:bg-toggle-on',
        props.class,
      )
    "
  >
    <SwitchThumb
      data-slot="switch-thumb"
      class="pointer-events-none block size-2.5 rounded-full bg-toggle-knob transition-transform data-[state=unchecked]:translate-x-0.5 data-[state=checked]:translate-x-3 data-[state=checked]:bg-toggle-mark"
    />
  </SwitchRoot>
</template>
