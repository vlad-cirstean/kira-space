<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { computed, useSlots, useTemplateRef } from 'vue';

// The one status bar item shape: icon, label, optional tooltip body (slot `tip`). Muted unless
// toned. Attributes (`data-testid`, `@click`) fall through to the Button; the button element is
// exposed as `$el` so a `PopoverTrigger as-child` can anchor on it, as TooltipIconButton does.
defineOptions({ inheritAttrs: false });

withDefaults(
  defineProps<{
    icon: string;
    tone?: 'info' | 'warn' | 'error';
    as?: string;
    disabled?: boolean;
  }>(),
  { tone: undefined, as: 'button', disabled: false },
);

const slots = useSlots();
const hasTip = computed(() => Boolean(slots.tip));
const buttonRef = useTemplateRef<{ $el: HTMLElement }>('buttonRef');
defineExpose({ $el: computed(() => buttonRef.value?.$el) });
</script>

<template>
  <Tooltip v-if="hasTip">
    <TooltipTrigger as-child>
      <TooltipDisabledTrigger v-if="disabled" :disabled="true">
        <Button
          ref="buttonRef"
          variant="toolbar"
          size="status"
          :as="as"
          :data-tone="tone"
          v-bind="$attrs"
        >
          <CodiconIcon :name="icon" :size="12" />
          <slot />
        </Button>
      </TooltipDisabledTrigger>
      <Button
        v-else
        ref="buttonRef"
        variant="toolbar"
        size="status"
        :as="as"
        :data-tone="tone"
        v-bind="$attrs"
      >
        <CodiconIcon :name="icon" :size="12" />
        <slot />
      </Button>
    </TooltipTrigger>
    <TooltipContent><slot name="tip" /></TooltipContent>
  </Tooltip>
  <Button
    v-else
    ref="buttonRef"
    variant="toolbar"
    size="status"
    :as="as"
    :disabled="disabled"
    :data-tone="tone"
    v-bind="$attrs"
  >
    <CodiconIcon :name="icon" :size="12" />
    <slot />
  </Button>
</template>
