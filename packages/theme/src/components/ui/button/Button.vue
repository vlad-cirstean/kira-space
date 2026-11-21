<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import ShortcutKbd from '@theme/components/ShortcutKbd.vue';
import Spinner from '@theme/components/Spinner.vue';
import { cn } from '@theme/lib/utils';
import { ariaKeyShortcuts } from '@theme/shortcutText';
import type { PrimitiveProps } from 'reka-ui';
import { Primitive } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import type { ButtonVariants } from '.';
import { buttonVariants } from '.';

interface Props extends PrimitiveProps {
  variant?: ButtonVariants['variant'];
  size?: ButtonVariants['size'];
  /** Tone of a `dialog` outline (border, text, focus outline). */
  outline?: ButtonVariants['outline'];
  class?: HTMLAttributes['class'];
  /** An async action is running: disabled, `aria-busy`, and a spinner replaces the button's icon. */
  pending?: boolean;
  /** Registry chord this button runs, shown inline after the label (dialog footers). */
  shortcut?: ShortcutId;
}

const props = withDefaults(defineProps<Props>(), {
  as: 'button',
});
</script>

<template>
  <Primitive
    data-slot="button"
    :data-variant="variant"
    :data-size="size"
    :as="as"
    :as-child="asChild"
    :disabled="pending || undefined"
    :aria-busy="pending || undefined"
    :aria-keyshortcuts="shortcut && ariaKeyShortcuts(shortcut)"
    :class="
      cn(
        buttonVariants({ variant, size, outline }),
        pending && '[&_.codicon:not(.animate-spin)]:hidden',
        props.class,
      )
    "
  >
    <Spinner v-if="pending && !asChild" label="" />
    <slot />
    <ShortcutKbd v-if="shortcut && !asChild" :id="shortcut" />
  </Primitive>
</template>
