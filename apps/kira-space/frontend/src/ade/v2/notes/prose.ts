/** Prose styling of a note, editable or read-only: one class string, so the two never drift. */
export function noteProse(flush: boolean | undefined): string {
  return [
    `ade-notes-prose kira-prose min-h-full ${flush ? 'p-0' : 'px-3 py-2'} outline-none`,
    '[&_p:last-child]:mb-0',
    "[&_ul[data-type='taskList']]:list-none [&_ul[data-type='taskList']]:pl-[0.2em]",
    "[&_ul[data-type='taskList']_li]:flex [&_ul[data-type='taskList']_li]:items-start [&_ul[data-type='taskList']_li]:gap-[0.4em]",
    "[&_input[type='checkbox']]:accent-ok",
    "[&_li[data-checked='true']>div]:text-subtle [&_li[data-checked='true']>div]:line-through",
    '[&_p.is-editor-empty:first-child]:before:pointer-events-none [&_p.is-editor-empty:first-child]:before:float-left [&_p.is-editor-empty:first-child]:before:h-0 [&_p.is-editor-empty:first-child]:before:text-subtle [&_p.is-editor-empty:first-child]:before:content-[attr(data-placeholder)]',
  ].join(' ');
}
