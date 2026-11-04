import { rowIndent, rowMeta } from '@theme/lib/rowVariants';

export { rowIndent, rowMeta };
// One task-panel row: lead | label? | body | actions, each cell aligned to the first line.
export const ADE_ROW = 'flex min-h-row min-w-0 items-start gap-1 pr-1.5 text-kira-md text-fg';
export const ADE_ROW_LEAD = 'relative flex h-row w-5 shrink-0 items-center justify-center';
export const ADE_ROW_LABEL =
  'flex h-row w-24 shrink-0 items-center text-kira-sm font-normal text-muted-foreground';
export const ADE_ROW_BODY = 'flex min-h-row min-w-0 flex-1 items-center gap-1.5';
export const ADE_ROW_ACTIONS = 'flex h-row shrink-0 items-center gap-1';
/** A text state glyph (✓ ● ! ✕) in the lead cell; callers add the tone or fill. */
export const ADE_ROW_GLYPH =
  'box-border flex size-4 items-center justify-center rounded-kira-sm text-kira-sm font-medium';
/** A status word right after the primary text; callers add the tone text class. */
export const ADE_ROW_STATUS = 'shrink-0 whitespace-nowrap text-kira-sm';
