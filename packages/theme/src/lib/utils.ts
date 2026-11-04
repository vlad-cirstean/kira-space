import { type ClassValue, clsx } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

// tailwind-merge's default config has no notion of base.css's kira-* scale steps, so it
// misreads e.g. `text-kira-sm` as a text *colour* utility and never dedupes it against a real
// colour class, or against `text-kira-md` from the same scale. Registering the real groups here
// (names taken from base.css's `@theme`) fixes both: kira-scale classes merge against their own
// scale, and stop masquerading as an unrelated group.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: [
        'kira-xs',
        'kira-sm',
        'kira-md',
        'kira-lg',
        'kira-xl',
        'kira-touch',
        'graph-xs',
        'graph-sm',
        'graph-md',
        'graph-lg',
      ],
      spacing: [
        'control',
        'control-lg',
        'control-sm',
        'control-inline',
        'row',
        'board-row',
        'board-row-tall',
        'bar',
        'graph-row',
        'graph-row-compact',
        'graph-h-xs',
        // P110 B6
        'titlebar',
        'tabbar',
        'statusbar',
        'titlebar-inset',
        // P110 I2-2: M2 -- max-w-completion-max-w was unregistered, so a static max-w utility
        // next to it on the same element read as a false conflict.
        'completion-max-w',
        'touch',
      ],
      radius: ['kira-xs', 'kira-sm', 'kira'],
      shadow: ['kira'],
    },
  },
});

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
