import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Card } from './Card.vue';
export { default as CardAction } from './CardAction.vue';
export { default as CardContent } from './CardContent.vue';
export { default as CardFooter } from './CardFooter.vue';
export { default as CardHeader } from './CardHeader.vue';
export { default as CardTitle } from './CardTitle.vue';

// P263 §2.10. `inset`: a box inside a form, dialog or pane; holds text only, its `bg-field` fill
// hides an Input, Select or Checkbox. `section`: a titled block in a detail pane; holds controls.
// `list`: a box of rows on the elevated fill. No variant draws a border. Children read the variant through the `group/card` data attribute.
export const cardVariants = cva('group/card min-w-0', {
  variants: {
    variant: {
      inset: 'flex flex-col gap-1.5 rounded-kira bg-field px-2.5 py-2',
      section: 'flex flex-col rounded-kira bg-elevated',
      list: 'flex flex-col rounded-kira bg-elevated p-1',
    },
  },
  defaultVariants: { variant: 'inset' },
});

export type CardVariants = VariantProps<typeof cardVariants>;
