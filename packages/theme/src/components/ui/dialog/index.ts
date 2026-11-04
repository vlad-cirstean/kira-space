import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Dialog } from '@theme/components/ui/dialog/Dialog.vue';
export { default as DialogBody } from '@theme/components/ui/dialog/DialogBody.vue';
export { default as DialogClose } from '@theme/components/ui/dialog/DialogClose.vue';
export { default as DialogContent } from '@theme/components/ui/dialog/DialogContent.vue';
export { default as DialogDescription } from '@theme/components/ui/dialog/DialogDescription.vue';
export { default as DialogFooter } from '@theme/components/ui/dialog/DialogFooter.vue';
export { default as DialogHeader } from '@theme/components/ui/dialog/DialogHeader.vue';
export { default as DialogOverlay } from '@theme/components/ui/dialog/DialogOverlay.vue';
export { default as DialogTitle } from '@theme/components/ui/dialog/DialogTitle.vue';
export { default as DialogTrigger } from '@theme/components/ui/dialog/DialogTrigger.vue';
export type { ConfirmDiscard, DiscardPrompt } from '@theme/components/ui/dialog/discard';
export { CONFIRM_DISCARD_KEY } from '@theme/components/ui/dialog/discard';

// P262 §2.2: shared class strings, also consumed by TextPromptDialog (non-portaled, own scrim).
export const dialogHeaderClass = 'flex flex-row items-center gap-1.5 px-3 py-2';
export const dialogBodyClass = 'flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3';
export const dialogFooterClass = 'flex items-center justify-end gap-1.5 px-3 py-2';

export const dialogContentSizeVariants = cva(
  'flex flex-col gap-0 p-0 max-w-[90vw] max-h-4/5 overflow-hidden',
  {
    variants: {
      size: {
        sm: 'w-100',
        md: 'w-120',
        lg: 'w-150',
        xl: 'w-180',
        '2xl': 'w-225',
      },
      fixedHeight: {
        true: 'h-140 max-h-[85vh]',
        false: '',
      },
    },
    compoundVariants: [{ size: '2xl', fixedHeight: true, class: 'h-160' }],
    defaultVariants: {
      fixedHeight: false,
    },
  },
);

export type DialogContentSizeVariants = VariantProps<typeof dialogContentSizeVariants>;
