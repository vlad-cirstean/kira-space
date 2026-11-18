<script setup lang="ts">
import { Kbd } from '@theme/components/ui/kbd';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import VarText from '@theme/components/VarText.vue';
import type { TextPart } from '@theme/varText';
import type { TooltipContentProps } from 'reka-ui';

// Plain-text tooltip; renders the bare slot when text and parts are empty.
defineProps<{
  text?: string | null;
  parts?: readonly TextPart[];
  side?: TooltipContentProps['side'];
  contentTestid?: string;
  /** Shortcut text (callers format it), shown as a `Kbd` after the text. */
  kbd?: string;
}>();
</script>

<template>
  <Tooltip v-if="text || parts?.length">
    <TooltipTrigger as-child>
      <slot />
    </TooltipTrigger>
    <TooltipContent :side="side" :data-testid="contentTestid"
      ><VarText v-if="parts?.length" :parts="parts" />
      <template v-else>{{ text }}</template><Kbd v-if="kbd">{{ kbd }}</Kbd></TooltipContent
    >
  </Tooltip>
  <slot v-else />
</template>
