import { badgeVariants } from '@theme/components/ui/badge/variants';
import { changeRequestTone, TONE_BADGE } from '@theme/lib/statusTone';
import { cn } from '@theme/lib/utils';

export type RefBadgeVariant = 'default' | 'ok' | 'info' | 'warn' | 'error' | 'chip';

// Studio Badge tones; only height and font follow Settings > Git graph font size.
export function refBadgeClass(variant: RefBadgeVariant = 'default', ...extra: string[]): string {
  return cn(badgeVariants({ variant }), 'h-graph-h-xs font-data text-graph-sm', ...extra);
}

/** `refBadges.ts` marks the graph PR badge with `data-testid`. */
export function changeRequestBadgeClass(state: string): string {
  return refBadgeClass(TONE_BADGE[changeRequestTone(state)], 'no-underline cursor-pointer');
}
