import type { ShortcutId } from '@shared/domain/shortcuts';
import type { TextPart } from '@theme/varText';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { useFocusReturn } from '../behaviour/focus';

/** One option of a `radio`; selecting it runs `run`. Reselecting the current one is a no-op unless `rerun`. */
export interface MenuRadioOption {
  type: 'option';
  id: string;
  /** Host `data-testid`; defaults to `menu-item-${id}`. */
  testId?: string;
  /** Compared with the radio's `value`; unique within the radio. `null` is the "none" option. */
  value: string | null;
  label: string;
  /** Rich label (VarText) rendered in place of `label`; `label` carries its plain text. */
  parts?: readonly TextPart[];
  /** A second, muted line under the label. */
  detail?: string;
  /** Renders a small color swatch in the icon column. */
  swatch?: string;
  disabled?: boolean;
  /** Run even when this option is the radio's current value. */
  rerun?: boolean;
  /** Hover explanation, also the reason a `disabled` option is blocked. */
  hint?: string | readonly TextPart[];
  run(): void | Promise<void>;
}

export type MenuItem =
  | {
      type: 'item';
      id: string;
      /** Host `data-testid`; defaults to `menu-item-${id}`. May repeat across items. */
      testId?: string;
      label: string;
      /** Rich label (VarText) rendered in place of `label`; `label` carries its plain text. */
      parts?: readonly TextPart[];
      /** A second, muted line under the label. */
      detail?: string;
      icon?: string;
      /** Renders a small color swatch instead of an icon — used by the Color submenu. */
      swatch?: string;
      danger?: boolean;
      disabled?: boolean;
      /** A hover explanation for this one row (e.g. the cell editor's format picker) —
       *  rendered through the same Tooltip/TooltipTrigger/TooltipContent trio every other hint
       *  uses, so it inherits the shared open-delay/rearm behavior for free rather than
       *  the row inventing its own. */
      hint?: string | readonly TextPart[];
      /** P21: names a binding in shared/domain/shortcuts.ts by id, never a display string — a typo is a
       *  type error, and the printed key can never drift from the key that actually runs `run()`. */
      shortcut?: ShortcutId;
      run(): void | Promise<void>;
    }
  | {
      type: 'submenu';
      id: string;
      testId?: string;
      label: string;
      icon?: string;
      items: MenuItem[];
    }
  | { type: 'separator' }
  /** One group of mutually exclusive options; `value` (null: none) marks the current one. Draws the
   *  primitive's check on it. Separators between option runs stay inside the group. */
  | {
      type: 'radio';
      id: string;
      value: string | null;
      items: readonly (MenuRadioOption | { type: 'separator' })[];
    }
  /** An on/off row; draws the primitive's check when `checked`. */
  | {
      type: 'checkbox';
      id: string;
      testId?: string;
      label: string;
      icon?: string;
      checked: boolean;
      disabled?: boolean;
      hint?: string | readonly TextPart[];
      /** Keeps the menu open on toggle (a facet list toggled several times in a row). */
      keepOpen?: boolean;
      run(): void | Promise<void>;
    }
  /** A non-interactive heading row above the items that follow it (P129 Part 5 §0.10's day
   *  context menu: the day's own `dayLong` above its one toggle). */
  | { type: 'label'; label: string };

export interface ContextMenuOptions {
  /** Runs on close when the invoking element is gone (a virtual row that scrolled away); focus the
   *  owning list instead. */
  onRestoreFocus?: () => void;
}

export const useContextMenuStore = defineStore('contextMenu', () => {
  const state = reactive({
    open: false,
    x: 0,
    y: 0,
    items: [] as MenuItem[],
  });
  const focusReturn = useFocusReturn();
  let onRestoreFocus: (() => void) | undefined;
  const pendingAfterClose: (() => void)[] = [];

  // P83 §9.2: a point, not an event — a dropdown anchored under a button (TabStrip.vue's "+") has
  // no MouseEvent of its own to read clientX/clientY from. openContextMenu below is now this plus
  // one destructure.
  function openContextMenuAt(
    x: number,
    y: number,
    items: MenuItem[],
    options?: ContextMenuOptions,
  ): void {
    if (!state.open) focusReturn.capture();
    onRestoreFocus = options?.onRestoreFocus;
    state.items = items;
    state.x = x;
    state.y = y;
    state.open = true;
  }

  function openContextMenu(ev: MouseEvent, items: MenuItem[], options?: ContextMenuOptions): void {
    openContextMenuAt(ev.clientX, ev.clientY, items, options);
  }

  /** Keyboard open (ContextMenu key, Shift+F10): anchored at the element's bottom-left. */
  function openContextMenuFor(el: Element, items: MenuItem[], options?: ContextMenuOptions): void {
    const rect = el.getBoundingClientRect();
    openContextMenuAt(rect.left, rect.bottom, items, options);
  }

  function closeContextMenu(): void {
    state.open = false;
  }

  /** Called by the menu on close: focus goes back to the element that held it at open. */
  function restoreFocus(): void {
    const fallback = onRestoreFocus;
    onRestoreFocus = undefined;
    focusReturn.restore(fallback);
  }

  /** Runs `fn` once the menu has closed and given focus back. For an item that moves focus itself
   *  (inline rename): the open menu traps focus, and its close would take it back from the field. */
  function afterClose(fn: () => void): void {
    if (state.open) pendingAfterClose.push(fn);
    else fn();
  }

  /** Called by the menu once its close has finished. */
  function runAfterClose(): void {
    const pending = pendingAfterClose.splice(0);
    for (const fn of pending) fn();
  }

  return {
    ...toRefs(state),
    openContextMenuAt,
    openContextMenu,
    openContextMenuFor,
    closeContextMenu,
    restoreFocus,
    afterClose,
    runAfterClose,
  };
});

// A new local keybinding dispatches through the same menu-builder function a right-click
// would call, rather than a parallel handler — the printed shortcut and the executed action are
// then the same object, and `disabled` gating (canEdit, a missing record, …) is honoured for
// free instead of being restated at the keydown site. Walks one level into submenus (e.g.
// `copy-rows-tsv` lives inside `rowMenu`'s "Copy row(s)" submenu).
function findShortcutItem(items: MenuItem[], id: ShortcutId): MenuItem | undefined {
  for (const item of items) {
    if (item.type === 'item' && item.shortcut === id && !item.disabled) return item;
    if (item.type === 'submenu') {
      const sub = item.items.find((s) => s.type === 'item' && s.shortcut === id && !s.disabled);
      if (sub) return sub;
    }
  }
  return undefined;
}

export function runMenuShortcut(items: MenuItem[], id: ShortcutId): boolean {
  const item = findShortcutItem(items, id);
  if (item?.type !== 'item') return false;
  void item.run();
  return true;
}
