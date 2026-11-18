<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@theme/components/ui/collapsible';
import { t } from '@theme/i18n';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// A collapsible block with a chevron toggle. Replaces `<details>`; tree rows keep `TreeTwisty`.
// `meta` styles a secondary disclosure under a block. Content spacing is built in.
// `label` is the trigger text; the `label` slot replaces it. Without either, the toggle is the
// chevron alone and carries a generic accessible name.
const props = defineProps<{
  label?: string;
  meta?: boolean;
  class?: HTMLAttributes['class'];
}>();

const open = defineModel<boolean>('open', { default: false });
</script>

<template>
  <Collapsible v-model:open="open" data-slot="disclosure" :class="props.class">
    <CollapsibleTrigger
      :class="
        cn(
          'flex w-full min-w-0 cursor-default items-center gap-1 text-left',
          meta ? 'text-kira-sm text-muted-foreground' : 'text-kira-md',
        )
      "
      :aria-label="label || $slots.label ? undefined : t('theme.disclosure.toggle.label')"
    >
      <CodiconIcon :name="open ? 'chevron-down' : 'chevron-right'" :size="12" class="shrink-0" />
      <slot name="label">{{ label }}</slot>
    </CollapsibleTrigger>
    <CollapsibleContent class="flex flex-col gap-2 pt-1">
      <slot />
    </CollapsibleContent>
  </Collapsible>
</template>
