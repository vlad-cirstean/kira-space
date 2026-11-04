import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Input } from '@theme/components/ui/input/Input.vue';

// P117 A2: a size axis. `kira-lg` (26px, default) is the dialog/form density, `kira` (22px) the
// toolbar density.
export const inputVariants = cva(
  'border-border-strong aria-invalid:border-error rounded-kira-sm border bg-field transition-colors text-kira-md file:h-6 file:text-kira-md file:font-medium w-full min-w-0 file:inline-flex file:border-0 file:bg-transparent file:text-fg placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-default disabled:opacity-50 read-only:border-transparent read-only:bg-transparent',
  {
    variants: {
      size: {
        kira: 'h-control px-2 py-0',
        'kira-lg': 'h-control-lg px-2 py-0',
        touch: 'h-touch px-3 py-0 text-kira-touch',
      },
      // `inline`: bare until focus, then the field border.
      appearance: {
        default: '',
        inline:
          'border-transparent bg-transparent read-only:border-transparent focus-visible:border-border-strong',
      },
    },
    defaultVariants: {
      size: 'kira-lg',
      appearance: 'default',
    },
  },
);

export type InputVariants = VariantProps<typeof inputVariants>;
