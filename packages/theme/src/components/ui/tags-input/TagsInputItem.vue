<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { TagsInputItemProps } from 'reka-ui';
import { TagsInputItem, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<TagsInputItemProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardProps(delegatedProps);
</script>

<template>
  <TagsInputItem
    data-slot="tags-input-item"
    v-bind="forwarded"
    :class="
      cn(
        'flex items-center gap-1 rounded-kira-sm border border-border bg-fg/10 px-1.5 font-data text-kira-sm data-[state=active]:border-primary',
        props.class,
      )
    "
  >
    <slot />
  </TagsInputItem>
</template>
