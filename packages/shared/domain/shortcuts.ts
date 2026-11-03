// The one table the context menu's displayed shortcut text (formatShortcut) and the local
// DOM-scoped keydown handlers (matchesShortcut) both read by id, so a displayed shortcut and the
// one that runs cannot drift. The native menu accelerators are Go's own copy
// (apps/*/internal/shell/accel.go).
export interface Chord {
  /** Key name: 'C', 'F2', 'Return', 'Delete', 'Backspace', 'Tab', ','. */
  key: string;
  cmdOrCtrl?: true;
  /** Literal Control on every platform, unlike cmdOrCtrl — what Control+Tab needs. */
  ctrl?: true;
  shift?: true;
  alt?: true;
}

/** Where a chord is live. A chord may repeat across scopes (Cmd+C copies a tree name in `tree`, a cell
 *  in `grid`), never inside one scope or between `global` and any other. */
export type ShortcutScope = 'global' | 'view' | 'tree' | 'grid' | 'form' | 'editor' | 'list';

export interface Binding {
  chord: Chord;
  scope: ShortcutScope;
  /** Platform override, read by the local keydown matcher. */
  mac?: Chord;
}

export const SHORTCUTS = {
  'app.settings': { scope: 'global', chord: { key: ',', cmdOrCtrl: true } },
  'app.newConnection': { scope: 'global', chord: { key: 'N', cmdOrCtrl: true } },
  'view.toggleProjectPanel': { scope: 'global', chord: { key: 'B', cmdOrCtrl: true } },
  'view.toggleOperationsPanel': { scope: 'global', chord: { key: 'J', cmdOrCtrl: true } },
  'view.commandPalette': { scope: 'global', chord: { key: 'P', cmdOrCtrl: true, shift: true } },
  'view.find': { scope: 'view', chord: { key: 'F', cmdOrCtrl: true } },
  'view.refresh': { scope: 'view', chord: { key: 'F5' } },
  'view.run': { scope: 'view', chord: { key: 'Return', cmdOrCtrl: true } },
  'view.runAll': { scope: 'view', chord: { key: 'Return', cmdOrCtrl: true, shift: true } },
  // P13 D7: VS Code's own Format Document chord — ⌥⇧F on macOS — so it's the one a user already
  // has in their fingers.
  'view.format': { scope: 'view', chord: { key: 'F', shift: true, alt: true } },
  'tab.next': { scope: 'global', chord: { key: 'Tab', ctrl: true } },
  'tab.prev': { scope: 'global', chord: { key: 'Tab', ctrl: true, shift: true } },
  'tab.close': { scope: 'global', chord: { key: 'W', cmdOrCtrl: true } },
  'window.new': { scope: 'global', chord: { key: 'N', cmdOrCtrl: true, shift: true } },
  'window.close': { scope: 'global', chord: { key: 'W', cmdOrCtrl: true, shift: true } },

  // Review window files list: next/previous file still needing a review.
  'review.nextUnreviewed': { scope: 'view', chord: { key: 'F8', alt: true } },
  'review.previousUnreviewed': { scope: 'view', chord: { key: 'F8', alt: true, shift: true } },

  'ade.reviewCode': { scope: 'global', chord: { key: 'R', cmdOrCtrl: true, shift: true } },

  'grid.copy': { scope: 'grid', chord: { key: 'C', cmdOrCtrl: true } },
  'grid.paste': { scope: 'grid', chord: { key: 'V', cmdOrCtrl: true } },
  'grid.edit': { scope: 'grid', chord: { key: 'Return' } },
  'grid.duplicateRows': { scope: 'grid', chord: { key: 'D', cmdOrCtrl: true } },
  'grid.deleteRows': {
    scope: 'grid',
    chord: { key: 'Delete' },
    mac: { key: 'Backspace', cmdOrCtrl: true },
  },

  'form.submit': { scope: 'form', chord: { key: 'Return', cmdOrCtrl: true } },
  'form.cancel': { scope: 'form', chord: { key: 'Escape' } },
  'form.save': { scope: 'form', chord: { key: 'S', cmdOrCtrl: true } },
  // The ContextMenu key opens the same menu (matched alongside in shortcutFor).
  'menu.open': { scope: 'view', chord: { key: 'F10', shift: true } },

  // Keyboard reorder of a sortable list: Alt+arrow moves the focused item.
  'list.moveUp': { scope: 'list', chord: { key: 'ArrowUp', alt: true } },
  'list.moveDown': { scope: 'list', chord: { key: 'ArrowDown', alt: true } },
  'list.moveLeft': { scope: 'list', chord: { key: 'ArrowLeft', alt: true } },
  'list.moveRight': { scope: 'list', chord: { key: 'ArrowRight', alt: true } },

  'editor.suggest': { scope: 'editor', chord: { key: 'Space', ctrl: true } },

  'tree.open': { scope: 'tree', chord: { key: 'Return' } },
  'tree.copyName': { scope: 'tree', chord: { key: 'C', cmdOrCtrl: true } },
  'tree.copyUri': {
    scope: 'tree',
    chord: { key: 'C', shift: true, alt: true },
    mac: { key: 'C', alt: true, cmdOrCtrl: true },
  },
  'tree.rename': { scope: 'tree', chord: { key: 'F2' } },
  'tree.selectAll': { scope: 'tree', chord: { key: 'A', cmdOrCtrl: true } },
  'tree.duplicate': { scope: 'tree', chord: { key: 'D', cmdOrCtrl: true } },
  'tree.delete': {
    scope: 'tree',
    chord: { key: 'Delete' },
    mac: { key: 'Backspace', cmdOrCtrl: true },
  },
} satisfies Record<string, Binding>;

export type ShortcutId = keyof typeof SHORTCUTS;
