// Plan task list column tracks: every card head, branch row, span and history row shares them, so
// columns line up whatever the content. Widths go on wrapper cells, never on shared components.
export const TASK_LEAD = 'inline-flex w-6 shrink-0 justify-center';
/** Line 2 lead: the task status, at the title x. */
export const TASK_STATUS_CELL = 'flex w-20 shrink-0 min-w-0 items-center overflow-hidden';
export const TASK_ACTION_CELL = 'flex w-control shrink-0 items-center justify-center';
export const TASK_STEP_CELL = 'flex w-28 shrink-0 min-w-0 items-center gap-1 @lg:w-48';
/** Estimate: spans the branch rows' PR and tag slots, right-aligned to the card edge. */
export const TASK_TIME_CELL =
  'flex w-[calc(var(--spacing-control)*2+--spacing(1.5))] shrink-0 min-w-0 items-center justify-end overflow-hidden';
export const BRANCH_GRID =
  'grid min-w-0 flex-1 grid-cols-[--spacing(20)_minmax(0,1fr)_auto_var(--spacing-control)_var(--spacing-control)] items-center gap-x-1.5 gap-y-0.5';
export const SLOT = 'flex w-control shrink-0 items-center justify-center';
/** Row ground for a hover overlay: the card sets it, a hovered or selected row overrides it. */
export const GROUND_CARD = '[--row-ground:var(--color-chrome)]';
/** Card or span row edge: the 1px neutral border. Task colour lives in the head band only. */
export const TASK_EDGE = 'border border-border';
/** Selected card or span row: 1.5px neutral outline inside the border, so no reflow. */
export const TASK_EDGE_SELECTED =
  'outline-[1.5px] outline-solid -outline-offset-[1.5px] outline-fg';
/** Card head ground: the task band, lifted on hover. The card sets `--task-band` / `--task-band-hover`. */
export const GROUND_HEAD =
  'bg-(--row-ground) [--row-ground:var(--task-band)] hover:[--row-ground:var(--task-band-hover)]';
/** Selected card head: the hover band held. */
export const GROUND_HEAD_SELECTED = 'bg-(--row-ground) [--row-ground:var(--task-band-hover)]';
export const GROUND_ROW = 'bg-(--row-ground) hover:[--row-ground:var(--color-field)]';
/** Selected branch row in a card: field ground and a 1px inset ring. */
export const GROUND_SELECTED =
  'bg-field [--row-ground:var(--color-field)] ring-1 ring-inset ring-fg';
/** Hover actions laid over the end of the text cell: no reflow when they appear. */
export const HOVER_OVERLAY =
  'absolute inset-y-0 right-0 items-center gap-0.5 pl-4 bg-linear-to-r from-transparent via-(--row-ground) via-35% to-(--row-ground)';
