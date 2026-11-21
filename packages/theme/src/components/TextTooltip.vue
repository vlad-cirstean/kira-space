<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import ShortcutKbd from '@theme/components/ShortcutKbd.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import VarText from '@theme/components/VarText.vue';
import { ariaKeyShortcuts } from '@theme/shortcutText';
import type { TextPart } from '@theme/varText';
import type { TooltipContentProps } from 'reka-ui';

// Plain-text tooltip; renders the bare slot when text, parts and shortcut are empty.
defineProps<{
  text?: string | null;
  parts?: readonly TextPart[];
  side?: TooltipContentProps['side'];
  contentTestid?: string;
  /** Registry chord the wrapped control runs: a `ShortcutKbd` after the text, `aria-keyshortcuts` on the trigger. */
  shortcut?: ShortcutId;
}>();
</script>

<template>
  <Tooltip v-if="text || parts?.length || shortcut">
    <TooltipTrigger as-child :aria-keyshortcuts="shortcut && ariaKeyShortcuts(shortcut)">
      <slot />
    </TooltipTrigger>
    <TooltipContent :side="side" :data-testid="contentTestid"
      ><VarText v-if="parts?.length" :parts="parts" />
      <template v-else>{{ text }}</template>
      <ShortcutKbd v-if="shortcut" :id="shortcut" /></TooltipContent
    >
  </Tooltip>
  <slot v-else />
</template>
