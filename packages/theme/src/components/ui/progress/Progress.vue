<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { ProgressRootProps } from 'reka-ui';
import { ProgressIndicator, ProgressRoot } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = withDefaults(defineProps<ProgressRootProps & { class?: HTMLAttributes['class'] }>(), {
  modelValue: 0,
});

const delegatedProps = reactiveOmit(props, 'class');
</script>

<template>
  <ProgressRoot
    data-slot="progress"
    v-bind="delegatedProps"
    :class="
      cn(
        'bg-control-edge/50 h-1 rounded-full relative flex w-full items-center overflow-x-hidden',
        props.class,
      )
    "
  >
    <ProgressIndicator
      data-slot="progress-indicator"
      class="bg-on size-full flex-1 transition-all"
      :style="`transform: translateX(-${100 - (props.modelValue ?? 0)}%);`"
    />
  </ProgressRoot>
</template>
