<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import type { ButtonVariants } from '@theme/components/ui/button';
import { Button } from '@theme/components/ui/button';
import { DropdownMenu, DropdownMenuTrigger } from '@theme/components/ui/dropdown-menu';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';

// A primary action plus a chevron button that opens a menu of alternatives, sharing one border.
// Attrs (`data-testid`, `@click`) land on the primary button; the `menu` slot holds the
// `DropdownMenuContent`. `tooltip` explains the primary action.
defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    variant?: ButtonVariants['variant'];
    size?: 'kira' | 'kira-lg';
    disabled?: boolean;
    pending?: boolean;
    tooltip?: string;
    chevronLabel: string;
    chevronTestid?: string;
    class?: HTMLAttributes['class'];
  }>(),
  { variant: 'toolbar', size: 'kira', tooltip: undefined, chevronTestid: undefined },
);

const open = defineModel<boolean>('open', { default: false });
</script>

<template>
  <div data-slot="split-button" :class="cn('inline-flex', props.class)">
    <TextTooltip v-if="tooltip" :text="tooltip"
      ><Button
        :variant="variant"
        :size="size"
        class="rounded-r-none"
        :disabled="disabled"
        :pending="pending"
        v-bind="$attrs"
      >
        <slot />
      </Button></TextTooltip
    >
    <Button
      v-else
      :variant="variant"
      :size="size"
      class="rounded-r-none"
      :disabled="disabled"
      :pending="pending"
      v-bind="$attrs"
    >
      <slot />
    </Button>
    <DropdownMenu v-model:open="open">
      <DropdownMenuTrigger as-child>
        <Button
          :variant="variant"
          :size="size === 'kira-lg' ? 'kira-lg' : 'kira-icon'"
          :class="cn('rounded-l-none border-l-0 px-0.5', size === 'kira-lg' && 'w-control-lg')"
          :disabled="disabled || pending"
          :aria-label="chevronLabel"
          :data-testid="chevronTestid"
        >
          <CodiconIcon name="chevron-down" :size="13" />
        </Button>
      </DropdownMenuTrigger>
      <slot name="menu" />
    </DropdownMenu>
  </div>
</template>
