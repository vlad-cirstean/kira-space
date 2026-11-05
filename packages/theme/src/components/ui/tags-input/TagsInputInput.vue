<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { TagsInputInputProps } from 'reka-ui';
import { TagsInputInput, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<TagsInputInputProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardProps(delegatedProps);
</script>

<template>
  <TagsInputInput
    data-slot="tags-input-input"
    v-bind="forwarded"
    :class="
      cn(
        'min-w-16 flex-1 border-0 bg-transparent p-0 font-data text-kira-md outline-none placeholder:text-muted-foreground',
        props.class,
      )
    "
  />
</template>
