<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { TagsInputRootEmits, TagsInputRootProps } from 'reka-ui';
import { TagsInputRoot, useForwardPropsEmits } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<TagsInputRootProps & { class?: HTMLAttributes['class'] }>();
const emits = defineEmits<TagsInputRootEmits>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <TagsInputRoot
    data-slot="tags-input"
    v-bind="forwarded"
    :class="
      cn(
        'flex min-h-control-lg w-full flex-wrap items-center gap-1 rounded-kira-sm border border-border bg-field px-1.5 py-0.5 text-kira-md transition-colors focus-within:border-primary aria-invalid:border-error data-[disabled]:opacity-50',
        props.class,
      )
    "
  >
    <slot />
  </TagsInputRoot>
</template>
