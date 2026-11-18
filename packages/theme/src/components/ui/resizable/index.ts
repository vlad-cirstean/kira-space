import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { RESIZE_KEY_STEP } from '@theme/components/ui/resizable/paneResizeMath';
export { default as ResizeHandle } from '@theme/components/ui/resizable/ResizeHandle.vue';

// The one pane-resize-handle look, bound by ResizeHandle. A 2px bar, hit area +-4px
// (+-8px coarse), grey `bg-resize-hover` on hover, `bg-resize-active` while dragging
// (`data-[state=drag]`), `bg-focus` on keyboard focus (the bar is the focus ring). `axis` x is a vertical bar (ew-resize), y a horizontal one (ns-resize). `line` draws
// the 1px divider between flat panes.
export const resizeHandleVariants = cva(
  'relative shrink-0 bg-transparent before:absolute hover:bg-resize-hover focus-visible:bg-focus focus-visible:outline-none data-[state=drag]:bg-resize-active',
  {
    variants: {
      axis: {
        x: 'w-0.5 cursor-ew-resize before:inset-y-0 before:-inset-x-1 pointer-coarse:before:-inset-x-2',
        y: 'h-0.5 cursor-ns-resize before:inset-x-0 before:-inset-y-1 pointer-coarse:before:-inset-y-2',
      },
      line: { true: '', false: '' },
    },
    compoundVariants: [
      {
        axis: 'x',
        line: true,
        class:
          'shadow-[inset_calc(var(--kira-border-width)*-1)_0_0_0_var(--kira-resize)] hover:shadow-none focus-visible:shadow-none data-[state=drag]:shadow-none',
      },
      {
        axis: 'y',
        line: true,
        class:
          'shadow-[inset_0_calc(var(--kira-border-width)*-1)_0_0_var(--kira-resize)] hover:shadow-none focus-visible:shadow-none data-[state=drag]:shadow-none',
      },
    ],
    defaultVariants: { axis: 'x', line: false },
  },
);

export type ResizeHandleVariants = VariantProps<typeof resizeHandleVariants>;
