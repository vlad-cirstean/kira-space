import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Tooltip } from '@theme/components/ui/tooltip/Tooltip.vue';
export { default as TooltipContent } from '@theme/components/ui/tooltip/TooltipContent.vue';
export { default as TooltipDisabledTrigger } from '@theme/components/ui/tooltip/TooltipDisabledTrigger.vue';
export { default as TooltipProvider } from '@theme/components/ui/tooltip/TooltipProvider.vue';
export { default as TooltipTrigger } from '@theme/components/ui/tooltip/TooltipTrigger.vue';

// Both variants share the elevated ground; `default` is inline text, `surface` is a wide column layout.
export const tooltipContentVariants = cva(
  'data-open:animate-in data-open:fade-in-0 data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-closed:animate-out data-closed:fade-out-0 duration-100 pointer-events-none z-(--kira-z-tooltip) rounded-kira bg-elevated text-fg border border-border shadow-kira text-kira-md break-words origin-(--reka-tooltip-content-transform-origin)',
  {
    variants: {
      variant: {
        default:
          'inline-flex items-center gap-1.5 px-2 py-1 leading-snug has-data-[slot=kbd]:pr-1 w-fit max-w-sm whitespace-pre-line',
        surface:
          'flex w-80 max-w-(--reka-tooltip-content-available-width) flex-col items-stretch gap-1.5 p-2',
      },
    },
    defaultVariants: { variant: 'default' },
  },
);

export type TooltipContentVariants = VariantProps<typeof tooltipContentVariants>;
