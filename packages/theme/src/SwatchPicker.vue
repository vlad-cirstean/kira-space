<script setup lang="ts">
import type { OfferedColor } from '@shared/domain/color';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { cn } from '@theme/lib/utils';
import type { HTMLAttributes } from 'vue';
import { colourLabel, connSwatchClass } from './connColor';

// One colour choice: shadcn-vue radio group, a tooltip and a label per swatch. The tooltip trigger
// wraps a span: on the item itself its data-state would overwrite the radio's checked state. A host `aria-label`
// falls through to the group; `labelledby` names it from a visible FieldLabel instead.
const props = withDefaults(
  defineProps<{
    colors: readonly string[];
    swatchClass?: (color: string) => string;
    colorLabel?: (color: string) => string;
    testidPrefix?: string;
    labelledby?: string;
    class?: HTMLAttributes['class'];
  }>(),
  {
    swatchClass: connSwatchClass,
    colorLabel: (color: string) => colourLabel(color as OfferedColor),
    testidPrefix: 'color',
    labelledby: undefined,
    class: undefined,
  },
);
const model = defineModel<string>({ required: true });
</script>

<template>
  <RadioGroup
    :model-value="model"
    :class="cn('flex flex-wrap items-center gap-1 w-auto', props.class)"
    orientation="horizontal"
    :aria-labelledby="labelledby"
    @update:model-value="(v) => (model = String(v))"
  >
    <TextTooltip v-for="c in colors" :key="c" :text="colorLabel(c)">
      <span class="inline-flex">
        <RadioGroupItem
          :value="c"
          :aria-label="colorLabel(c)"
          :data-testid="`${testidPrefix}-${c}`"
          :class="
            cn(
              'size-4 border-transparent data-[state=checked]:border-transparent data-[state=checked]:outline-2 outline-offset-2 outline-fg',
              swatchClass(c),
            )
          "
        >
          <span />
        </RadioGroupItem>
      </span>
    </TextTooltip>
  </RadioGroup>
</template>
