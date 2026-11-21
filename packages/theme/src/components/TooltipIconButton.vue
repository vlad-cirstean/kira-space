<script setup lang="ts">
import type { ShortcutId } from '@shared/domain/shortcuts';
import type { CodiconSize } from '@theme/CodiconIcon.vue';
import CodiconIcon from '@theme/CodiconIcon.vue';
import ShortcutKbd from '@theme/components/ShortcutKbd.vue';
import type { ButtonVariants } from '@theme/components/ui/button';
import { Button } from '@theme/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipDisabledTrigger,
  TooltipTrigger,
} from '@theme/components/ui/tooltip';
import { ariaKeyShortcuts } from '@theme/shortcutText';
import { computed, useTemplateRef } from 'vue';

// 152 call sites across 52 files repeated this exact
// Tooltip > TooltipTrigger as-child > [TooltipDisabledTrigger] > Button > CodiconIcon shape, 13-16
// lines of template each. `label` doubles as the tooltip text and the button's `aria-label` (the
// overwhelming majority of sites use the same string for both); `ariaLabel` is an escape hatch for
// the few sites whose accessible name and tooltip text genuinely differ (a static aria-label next
// to a dynamic tooltip). The default slot overrides the tooltip body for a site whose tooltip is
// richer than plain text/an interpolation. The `badge` slot draws over the icon (a status dot). Every other attribute (`data-testid`, `@click`,
// `disabled`, extra classes) falls through to the Button via $attrs — inheritAttrs is off because
// this component has no single root element to inherit onto.
defineOptions({ inheritAttrs: false });

withDefaults(
  defineProps<{
    icon: string;
    label: string;
    ariaLabel?: string;
    iconSize?: CodiconSize;
    disabledTrigger?: boolean;
    variant?: ButtonVariants['variant'];
    size?: ButtonVariants['size'];
    /** Tone of a `dialog` outline; see Button. */
    outline?: ButtonVariants['outline'];
    /** `aria-pressed`; the toolbar variant draws the pressed look from it. */
    pressed?: boolean;
    /** Colours the icon for a state: `ok` (done, on), `warn` or `error`. */
    tone?: 'ok' | 'warn' | 'error' | 'info';
    /** Registry chord this button runs: a `ShortcutKbd` after the label, `aria-keyshortcuts` on the button. */
    shortcut?: ShortcutId;
    /** `data-testid` of the tooltip body. */
    contentTestid?: string;
  }>(),
  {
    ariaLabel: undefined,
    iconSize: 13,
    disabledTrigger: false,
    variant: 'toolbar',
    size: 'kira-icon',
    outline: undefined,
    pressed: undefined,
    tone: undefined,
    shortcut: undefined,
    contentTestid: undefined,
  },
);

// A handful of call sites (TimestampPane.vue's calendar trigger, FilterToolbar.vue's history
// trigger, StreamView.vue's ×3, DocumentView.vue's ×2) put `ref="x"` on this component expecting
// `x.value.$el` to be the real button DOM node — the pre-extraction inline Button carried that ref
// directly. This component's own root is `Tooltip`, which renders a second, teleported
// `TooltipContent` sibling alongside the trigger, so the built-in `$el` a single-root component
// gets for free does not reliably resolve to the button here. Re-exposed explicitly off the real
// template ref instead of relying on that built-in.
const buttonRef = useTemplateRef<{ $el: HTMLElement }>('buttonRef');
defineExpose({ $el: computed(() => buttonRef.value?.$el) });
</script>

<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <TooltipDisabledTrigger
        v-if="disabledTrigger"
        :disabled="$attrs.disabled !== undefined && $attrs.disabled !== false"
      >
        <Button
          ref="buttonRef"
          :variant="variant"
          :outline="outline"
          :size="size"
          :aria-label="ariaLabel ?? label"
          :aria-pressed="pressed"
          :aria-keyshortcuts="shortcut && ariaKeyShortcuts(shortcut)"
          :data-tone="tone"
          v-bind="$attrs"
        >
          <CodiconIcon :name="icon" :size="iconSize" />
          <slot name="badge" />
        </Button>
      </TooltipDisabledTrigger>
      <Button
        v-else
        ref="buttonRef"
        :variant="variant"
        :outline="outline"
        :size="size"
        :aria-label="ariaLabel ?? label"
        :aria-pressed="pressed"
        :aria-keyshortcuts="shortcut && ariaKeyShortcuts(shortcut)"
        :data-tone="tone"
        v-bind="$attrs"
      >
        <CodiconIcon :name="icon" :size="iconSize" />
        <slot name="badge" />
      </Button>
    </TooltipTrigger>
    <TooltipContent :data-testid="contentTestid"
      ><slot>{{ label }}</slot><ShortcutKbd v-if="shortcut" :id="shortcut" /></TooltipContent
    >
  </Tooltip>
</template>
