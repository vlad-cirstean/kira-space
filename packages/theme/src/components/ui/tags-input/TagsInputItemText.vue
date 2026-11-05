<script setup lang="ts">
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { TagsInputItemTextProps } from 'reka-ui';
import { TagsInputItemText, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<TagsInputItemTextProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardProps(delegatedProps);
</script>

<template>
  <TagsInputItemText
    data-slot="tags-input-item-text"
    v-bind="forwarded"
    :class="cn('py-0.5', props.class)"
  />
</template>
