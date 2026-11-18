import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

// P110 B11: replaces workbench.css's own `.field`/`.field.checkbox` pair (`.field` -> vertical,
// `.field.checkbox` -> horizontal). gap-1/gap-1.5 are --kira-s-2/--kira-s-3 (4px/6px).
export const fieldVariants = cva('text-kira-md', {
  variants: {
    orientation: {
      vertical: 'flex flex-col gap-1',
      horizontal: 'flex flex-row items-center gap-1.5 has-[>[data-slot=field-content]]:items-start',
      // Label column beside the control once the nearest @container is 24rem wide, stacked below.
      responsive:
        'grid grid-cols-1 gap-1 @[24rem]:grid-cols-[7.5rem_minmax(0,1fr)] @[24rem]:gap-x-3 @[24rem]:[&>[data-slot$=label]]:flex @[24rem]:[&>[data-slot$=label]]:min-h-control-lg @[24rem]:[&>[data-slot$=label]]:items-center @[24rem]:[&>:not([data-slot$=label])]:col-start-2',
    },
  },
  defaultVariants: {
    orientation: 'vertical',
  },
});

export type FieldVariants = VariantProps<typeof fieldVariants>;

export { default as Field } from './Field.vue';
export { default as FieldContent } from './FieldContent.vue';
export { default as FieldDescription } from './FieldDescription.vue';
export { default as FieldError } from './FieldError.vue';
export { default as FieldGroup } from './FieldGroup.vue';
export { default as FieldLabel } from './FieldLabel.vue';
export { default as FieldLegend } from './FieldLegend.vue';
export { default as FieldSeparator } from './FieldSeparator.vue';
export { default as FieldSet } from './FieldSet.vue';
export { default as FieldTitle } from './FieldTitle.vue';
