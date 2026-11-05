<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import VarText from '@theme/components/VarText.vue';
import type { TextPart } from '@theme/varText';
import { computed } from 'vue';
import type { Tone } from '../board/actions';
import { STATUS_TEXT_CLASS } from '../statusColors';
import { SOLID_CLASS, TONE_TEXT_CLASS } from '../tones';
import type { StatusColor } from '../wire';
import type { Glyph } from './glyphs';

// A tag as a glyph: tone or status colour on the icon, a solid disc when urgent. The label and tip
// show in the tooltip; the label is the accessible name.
const props = defineProps<{
  glyph: Glyph;
  kind: string;
  label: string;
  tip: string;
  tipParts?: readonly TextPart[];
  tone?: Tone;
  color?: StatusColor;
}>();

const cls = computed(() => {
  const g = props.glyph;
  if (g.urgent) {
    return [
      'size-5 rounded-full inline-flex items-center justify-center',
      SOLID_CLASS[props.tone ?? 'red'],
    ];
  }
  if (props.color) return ['inline-flex', STATUS_TEXT_CLASS[props.color]];
  return ['inline-flex', TONE_TEXT_CLASS[props.tone ?? 'grey']];
});
</script>

<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <span
        role="img"
        class="shrink-0"
        :class="cls"
        :aria-label="label"
        data-testid="ade-tag"
        :data-kind="kind"
        :data-tone="color ? undefined : tone"
        :data-color="color"
        :data-icon="glyph.icon"
        :data-urgent="glyph.urgent || undefined"
      >
        <Spinner
          v-if="glyph.rotating"
          :icon="glyph.icon === 'sync' ? 'sync' : 'loading'"
          label=""
        />
        <CodiconIcon v-else :name="glyph.icon" :size="13" />
      </span>
    </TooltipTrigger>
    <TooltipContent>
      <div class="font-medium">{{ label }}</div>
      <div v-if="tipParts?.length || tip"
        ><VarText :parts="tipParts?.length ? tipParts : [tip]" /></div
      >
    </TooltipContent>
  </Tooltip>
</template>
