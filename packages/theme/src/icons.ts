/** The one concept-to-glyph table for UI icons (codicon names, no `codicon-` prefix). A concept
 *  that appears here uses this glyph everywhere; a kind of thing (a file, an engine) uses its own
 *  kind icon instead. Destroying stored data is `trash`; detaching from a list or selection is
 *  `close`. */
export const ICON = {
  add: 'add',
  delete: 'trash',
  remove: 'close',
  close: 'close',
  clearField: 'close',
  clearAll: 'clear-all',
  copy: 'copy',
  copied: 'check',
  duplicate: 'files',
  edit: 'edit',
  rename: 'edit',
  refresh: 'refresh',
  search: 'search',
  filter: 'filter',
  history: 'history',
  run: 'play',
  stop: 'debug-stop',
  restart: 'debug-restart',
  resume: 'debug-continue',
  takeOver: 'terminal',
  openInNewTab: 'empty-window',
  reset: 'discard',
  settings: 'settings-gear',
  moreActions: 'ellipsis',
  back: 'arrow-left',
  stepPrevious: 'chevron-left',
  stepNext: 'chevron-right',
  newCollection: 'new-folder',
  environments: 'server-environment',
  variables: 'variable-group',
  error: 'error',
  warning: 'warning',
  info: 'info',
  success: 'check',
} as const;

export type IconConcept = keyof typeof ICON;
