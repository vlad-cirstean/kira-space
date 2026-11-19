<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { colorMarkClass } from '@theme/connColor';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import ViewToolbar from './ViewToolbar.vue';

// A view's head: connection colour dot, kind icon, title with a dim prefix, badges (default slot),
// actions, then the colour band. `color` set draws the dot (an empty ring for no colour); the band
// always renders. `lead` adds a leading icon before the kind icon (an engine icon); `title` replaces
// the title text (a picker). Attrs (a `data-*` marker) go to the toolbar.
defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    color?: string;
    icon: string;
    iconClass?: string;
    prefix?: string;
    title?: string;
    testid?: string;
    titleTestid?: string;
    dotTestid?: string;
    bandTestid?: string;
  }>(),
  {
    color: undefined,
    iconClass: 'text-muted-foreground',
    prefix: undefined,
    title: undefined,
    testid: undefined,
    titleTestid: undefined,
    dotTestid: undefined,
    bandTestid: undefined,
  },
);

const fullTitle = computed(() => `${props.prefix ?? ''}${props.title ?? ''}`);
</script>

<template>
  <ViewToolbar v-bind="$attrs" :data-testid="testid">
    <span
      v-if="color !== undefined"
      :class="colorMarkClass('dot', color)"
      :data-testid="dotTestid"
    />
    <span v-if="$slots.lead" class="flex size-4 shrink-0 items-center justify-center"
      ><slot name="lead" /></span
    >
    <span :class="cn('flex size-4 shrink-0 items-center justify-center', iconClass)"
      ><CodiconIcon :name="icon" :size="13" /></span
    >
    <slot name="title"
      ><span class="min-w-0 text-kira-md text-fg" :data-testid="titleTestid"
        ><TruncatedText :text="fullTitle"
          ><span v-if="prefix" class="text-subtle">{{ prefix }}</span>{{ title }}</TruncatedText
        ></span
      ></slot
    >
    <slot />
    <div v-if="$slots.actions" class="ml-auto flex shrink-0 items-center gap-1">
      <slot name="actions" />
    </div>
  </ViewToolbar>
  <div :class="colorMarkClass('band', color)" :data-testid="bandTestid" />
</template>
