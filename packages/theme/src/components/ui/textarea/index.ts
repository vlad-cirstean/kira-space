import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Textarea } from '@theme/components/ui/textarea/Textarea.vue';

export const textareaVariants = cva(
  'border-border aria-invalid:border-error rounded-kira-sm border bg-field px-2 py-1.5 text-kira-md transition-colors flex field-sizing-content min-h-16 w-full placeholder:text-muted-foreground disabled:cursor-default disabled:opacity-50 read-only:border-border read-only:bg-transparent',
  {
    variants: {
      size: {
        default: '',
        touch: 'px-3 text-kira-touch',
      },
      // `inline`: bare until focus, then the field border.
      appearance: {
        default: '',
        inline:
          'border-transparent bg-transparent read-only:border-transparent focus-visible:border-border',
      },
    },
    defaultVariants: { size: 'default', appearance: 'default' },
  },
);

export type TextareaVariants = VariantProps<typeof textareaVariants>;
