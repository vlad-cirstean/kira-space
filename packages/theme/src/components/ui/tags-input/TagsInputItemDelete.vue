<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { cn } from '@theme/lib/utils';
import { reactiveOmit } from '@vueuse/core';
import type { TagsInputItemDeleteProps } from 'reka-ui';
import { TagsInputItemDelete, useForwardProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';

const props = defineProps<TagsInputItemDeleteProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardProps(delegatedProps);
</script>

<template>
  <TagsInputItemDelete
    data-slot="tags-input-item-delete"
    v-bind="forwarded"
    :class="cn('flex items-center text-muted-foreground hover:text-fg', props.class)"
  >
    <slot>
      <CodiconIcon name="close" :size="12" />
    </slot>
  </TagsInputItemDelete>
</template>
