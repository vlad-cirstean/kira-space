import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Toggle } from '@theme/components/ui/toggle/Toggle.vue';

export const toggleVariants = cva(
  'hover:text-fg aria-pressed:bg-field aria-invalid:border-error data-[state=on]:bg-field gap-1 rounded-kira-sm text-kira-md font-medium transition-all [&_svg:not([class*=size-])]:size-4 group/toggle hover:bg-field inline-flex items-center justify-center whitespace-nowrap disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-transparent',
        outline: 'border-border hover:bg-field border bg-transparent',
      },
      size: {
        default:
          'h-8 min-w-8 px-2.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        sm: 'h-7 min-w-7 rounded-kira-sm px-2.5 has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*=size-])]:size-3.5',
        lg: 'h-9 min-w-9 px-2.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        // P117 A1: the control-scale token size (Button's own kira precedent) — every pane/tab
        // ToggleGroup previously rendered at this cva's stock default (14px/32px) beside 11px/22px
        // toolbar chrome.
        kira: 'h-control min-w-control px-2 [&_svg:not([class*=size-])]:size-3.5',
        // P121 S5-S8: dialog density (mirrors Button's kira-lg) for a ToggleGroup rendered beside
        // dialog inputs/buttons, not toolbar chrome.
        'kira-lg': 'h-control-lg min-w-control-lg px-2 [&_svg:not([class*=size-])]:size-3.5',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

export type ToggleVariants = VariantProps<typeof toggleVariants>;
