<script setup lang="ts">
import { t } from '@theme/i18n';
import { TONE_TEXT } from '@theme/lib/statusTone';
import type { PrGlyphModel } from './prGlyph';

// The status lines of one pull request glyph, one element per line (never a joined string). `tinted`
// colours each by its tone (inline); a tooltip keeps the body colour.
defineProps<{
  lines: PrGlyphModel['lines'];
  /** `div` stacks the lines (tooltip); `span` runs them inline. */
  as?: 'div' | 'span';
  tinted?: boolean;
  /** Each line gets `data-testid="<prefix>-<part>"`. */
  linePrefix?: string;
}>();
</script>

<template>
  <component
    :is="as ?? 'span'"
    v-for="l in lines"
    :key="l.part"
    :class="tinted ? TONE_TEXT[l.tone] : undefined"
    :data-part="l.part"
    :data-testid="linePrefix ? `${linePrefix}-${l.part}` : undefined"
    >{{
      t(l.label)
    }}</component
  >
</template>
