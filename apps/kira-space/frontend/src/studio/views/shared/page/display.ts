// Display only: the cell editor, copy, search and export keep the full text. A cell holds up to
// MaxCellBytes (64 KiB); WebKit shapes a whole nowrap text run even when clipped, so hundreds of
// huge cells in the DOM freeze the renderer. 2 000 chars outrun any column a user can drag.
export const CELL_DISPLAY_CHARS = 2_000;

export function displayText(text: string): string {
  return text.length > CELL_DISPLAY_CHARS ? `${text.slice(0, CELL_DISPLAY_CHARS)}…` : text;
}
