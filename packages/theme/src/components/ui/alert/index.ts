import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Alert } from '@theme/components/ui/alert/Alert.vue';
export { default as AlertAction } from '@theme/components/ui/alert/AlertAction.vue';
export { default as AlertDescription } from '@theme/components/ui/alert/AlertDescription.vue';
export { default as AlertTitle } from '@theme/components/ui/alert/AlertTitle.vue';

// P263 §2.7: two-column grid (tone icon, content). Banner tint /10 on fill, no border; text uses
// the tone's readable token (`warn-text`, `note-text` for info). A grid block fills its row, so no
// width class; spacing between blocks belongs to the parent.
export const alertVariants = cva(
  'group/alert relative grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2 gap-y-0.5 rounded-kira px-2.5 py-2 text-left text-kira-md has-data-[slot=alert-action]:pr-18 *:col-start-2 *:data-[slot=alert-icon]:col-start-1 *:data-[slot=alert-icon]:row-start-1',
  {
    variants: {
      variant: {
        error:
          'bg-error/10 text-error *:data-[slot=alert-description]:text-error *:data-[slot=alert-icon]:text-error',
        warn: 'bg-warn/10 text-warn-text *:data-[slot=alert-description]:text-warn-text *:data-[slot=alert-icon]:text-warn',
        info: 'bg-info/10 text-note-text *:data-[slot=alert-description]:text-note-text *:data-[slot=alert-icon]:text-info',
      },
      strip: {
        true: 'rounded-none',
        false: '',
      },
    },
    defaultVariants: {
      variant: 'info',
      strip: false,
    },
  },
);

export type AlertVariants = VariantProps<typeof alertVariants>;

export const ALERT_ICONS = { error: 'error', warn: 'warning', info: 'info' } as const;
