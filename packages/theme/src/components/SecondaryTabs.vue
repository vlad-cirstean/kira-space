<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { ToggleGroup, ToggleGroupItem } from '@theme/components/ui/toggle-group';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
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
    variant?: 'tabs' | 'segmented' | 'text';
    size?: 'kira' | 'kira-lg';
    ariaLabel?: string;
    countBadge?: boolean;
  }>(),
  {
    variant: 'tabs',
    size: 'kira',
    ariaLabel: undefined,
    countBadge: false,
  },
);

// `text`: bare underlined words, no chip (approval content tabs); `tabs` adds muted idle text.
const TEXT_ITEM =
  'h-auto min-w-0 px-0 bg-transparent text-kira-sm font-normal text-subtle hover:bg-transparent data-[state=on]:bg-transparent data-[state=on]:text-fg data-[state=on]:underline data-[state=on]:decoration-primary data-[state=on]:underline-offset-3';

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
    :variant="variant === 'segmented' ? 'outline' : 'default'"
    :class="variant === 'text' && 'gap-3'"
    :size="size"
    :model-value="model"
    :aria-label="ariaLabel"
    @update:model-value="onUpdate"
  >
    <template v-for="item in items" :key="item.value">
      <!-- A tooltip trigger stamps its own data-state on the element it wraps, which would hide
           the toggle's on/off state: wrap in a span only when the item has a tooltip. -->
      <Tooltip v-if="item.tooltip">
        <TooltipTrigger as-child>
          <span class="inline-flex">
            <ToggleGroupItem
              :value="item.value"
              :disabled="item.disabled"
              :data-testid="item.testid"
              :class="
                cn(
                  variant === 'tabs' && 'text-muted-foreground data-[state=on]:text-fg',
                  variant === 'text' && TEXT_ITEM,
                )
              "
            >
              <slot name="item" :item="item" :active="item.value === model">
                <CodiconIcon v-if="item.icon" :name="item.icon" :size="13" />
                <span>{{ item.label }}</span>
                <template v-if="item.count !== undefined">
                  <Badge v-if="countBadge" variant="count" class="px-1">{{ item.count }}</Badge>
                  <span v-else class="text-kira-sm text-muted-foreground">{{ item.count }}</span>
                </template>
              </slot>
            </ToggleGroupItem>
          </span>
        </TooltipTrigger>
        <TooltipContent>{{ item.tooltip }}</TooltipContent>
      </Tooltip>
      <ToggleGroupItem
        v-else
        :value="item.value"
        :disabled="item.disabled"
        :data-testid="item.testid"
        :class="
          cn(
            variant === 'tabs' && 'text-muted-foreground data-[state=on]:text-fg',
            variant === 'text' && TEXT_ITEM,
          )
        "
      >
        <slot name="item" :item="item" :active="item.value === model">
          <CodiconIcon v-if="item.icon" :name="item.icon" :size="13" />
          <span>{{ item.label }}</span>
          <template v-if="item.count !== undefined">
            <Badge v-if="countBadge" variant="count" class="px-1">{{ item.count }}</Badge>
            <span v-else class="text-kira-sm text-muted-foreground">{{ item.count }}</span>
          </template>
        </slot>
      </ToggleGroupItem>
    </template>
  </ToggleGroup>
</template>
