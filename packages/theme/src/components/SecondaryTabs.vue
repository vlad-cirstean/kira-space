<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Badge } from '@theme/components/ui/badge';
import { ToggleGroup, ToggleGroupItem } from '@theme/components/ui/toggle-group';
import { cn } from '@theme/lib/utils';

// P262 §2.3: the one secondary-tab / segmented-picker element. Wraps ToggleGroup so call sites
// drop the `(v) => v && set(v)` guard: an empty value (clicking the active item) is never emitted.
export interface SecondaryTabItem {
  value: string;
  label: string;
  icon?: string;
  count?: number | string;
  disabled?: boolean;
  testid?: string;
  tooltip?: string;
}

withDefaults(
  defineProps<{
    items: readonly SecondaryTabItem[];
    variant?: 'tabs' | 'segmented' | 'text' | 'choice';
    size?: 'kira' | 'kira-lg';
    ariaLabel?: string;
    /** Spaced chips that wrap onto further rows instead of one connected strip. */
    wrap?: boolean;
  }>(),
  {
    variant: 'tabs',
    size: 'kira',
    ariaLabel: undefined,
    wrap: false,
  },
);

// `text`: bare underlined words, no chip (approval content tabs); idle text is muted by the Toggle base.
const TEXT_ITEM =
  'h-auto min-w-0 px-0 bg-transparent text-kira-sm font-normal text-subtle hover:bg-transparent data-[state=on]:bg-transparent data-[state=on]:text-fg data-[state=on]:underline data-[state=on]:decoration-select-edge data-[state=on]:underline-offset-3';

// `choice`: form single-select. Solid `bg-select` fill; `segmented` selected equals hover (`bg-field`).
const CHOICE_ITEM = 'max-w-full data-[state=on]:bg-select aria-pressed:bg-select';

const model = defineModel<string>({ required: true });

function onUpdate(v: unknown): void {
  if (typeof v === 'string' && v !== '') {
    model.value = v;
  }
}
</script>

<template>
  <ToggleGroup
    type="single"
    data-slot="secondary-tabs"
    :variant="variant === 'segmented' || variant === 'choice' ? 'outline' : 'default'"
    :spacing="wrap ? 1 : undefined"
    :class="cn(variant === 'text' && 'gap-3', wrap && 'flex-wrap')"
    :size="size"
    :model-value="model"
    :aria-label="ariaLabel"
    @update:model-value="onUpdate"
  >
    <template v-for="item in items" :key="item.value">
      <!-- A tooltip trigger stamps its own data-state on the element it wraps, which would hide
           the toggle's on/off state: wrap in a span only when the item has a tooltip. -->
      <TextTooltip v-if="item.tooltip" :text="item.tooltip"
        ><span class="inline-flex">
          <ToggleGroupItem
            :value="item.value"
            :disabled="item.disabled"
            :data-testid="item.testid"
            :class="cn(variant === 'text' && TEXT_ITEM, variant === 'choice' && CHOICE_ITEM)"
          >
            <slot name="item" :item="item" :active="item.value === model">
              <CodiconIcon v-if="item.icon" :name="item.icon" :size="13" />
              <span :class="variant === 'choice' && 'min-w-0 truncate'">{{ item.label }}</span>
              <template v-if="item.count !== undefined">
                <Badge variant="count" class="px-1">{{ item.count }}</Badge>
              </template>
            </slot>
          </ToggleGroupItem>
        </span></TextTooltip
      >
      <ToggleGroupItem
        v-else
        :value="item.value"
        :disabled="item.disabled"
        :data-testid="item.testid"
        :class="cn(variant === 'text' && TEXT_ITEM, variant === 'choice' && CHOICE_ITEM)"
      >
        <slot name="item" :item="item" :active="item.value === model">
          <CodiconIcon v-if="item.icon" :name="item.icon" :size="13" />
          <span :class="variant === 'choice' && 'min-w-0 truncate'">{{ item.label }}</span>
          <template v-if="item.count !== undefined">
            <Badge variant="count" class="px-1">{{ item.count }}</Badge>
          </template>
        </slot>
      </ToggleGroupItem>
    </template>
  </ToggleGroup>
</template>
