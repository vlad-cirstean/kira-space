import { cn } from '@theme/lib/utils';
import { VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';

// Pseudo line, not a border: a border adds height and drifts the virtualizer's computed offsets.
const LINE =
  'after:pointer-events-none after:absolute after:inset-x-0 after:bottom-0 after:h-px after:bg-grid-line';

/** Virtual row classes; `lined` adds the proof window's bottom line. */
export function virtualRowClass(lined: boolean | undefined, ...extra: string[]): string {
  return cn(VIRTUAL_ROW_CLASS, extra, lined && LINE);
}

/** Key-value row: the row line plus a divider right of the field cell. */
export function kvRowClass(lined: boolean | undefined): string {
  return virtualRowClass(
    lined,
    lined ? '[&>.cell:first-child]:border-r [&>.cell:first-child]:border-grid-line' : '',
  );
}
