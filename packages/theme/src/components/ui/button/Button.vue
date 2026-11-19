<script setup lang="ts">
import Spinner from '@theme/components/Spinner.vue';
import { Kbd } from '@theme/components/ui/kbd';
import { cn } from '@theme/lib/utils';
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
  /** Shortcut text (callers format it), shown as a Kbd after the label. */
  kbd?: string;
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
    <Kbd v-if="kbd && !asChild">{{ kbd }}</Kbd>
  </Primitive>
</template>
