/** The framed pane: a rounded, clipped box on the app background. */
export const paneFrame = 'min-h-0 min-w-0 overflow-hidden rounded-kira bg-bg';

/** Standalone window body: chrome gutter around framed panes. */
export const windowBody = 'flex min-h-0 flex-1 gap-0.5 bg-chrome px-1.5 pb-0.5';

/** Popover, menu and toast. */
export const floatingSurface = 'rounded-kira border border-border bg-elevated text-fg shadow-kira';

/** Dialog, command palette and text prompt: the modal surface. */
export const dialogSurface = 'rounded-kira border border-border bg-dialog text-fg shadow-kira';
