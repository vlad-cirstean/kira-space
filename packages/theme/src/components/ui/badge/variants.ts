import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

// One shape (h-control-sm, rounded-kira-sm, gap-1); colour varies per variant. Chip tints share one
// alpha (/16). `chip` is the untoned case (method/status hosts carrying their own colour class).
// The UI font is the default; `data` (Badge.vue) opts values that came from data into font-data.
export const badgeVariants = cva(
  'inline-flex h-control-sm shrink-0 items-center gap-1 whitespace-nowrap text-kira-sm [&>svg]:pointer-events-none [&>svg]:size-3',
  {
    variants: {
      variant: {
        default: 'rounded-kira-sm px-1 bg-field text-muted-foreground',
        chip: 'rounded-kira-sm px-1',
        ok: 'rounded-kira-sm px-1 bg-ok/16 text-ok',
        warn: 'rounded-kira-sm px-1 bg-warn/16 text-warn',
        error: 'rounded-kira-sm px-1 bg-error/16 text-error',
        info: 'rounded-kira-sm px-1 bg-info/16 text-info',
        count: 'min-w-control-sm justify-center rounded-kira-pill px-1.5 bg-field text-fg',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

export type BadgeVariants = VariantProps<typeof badgeVariants>;
