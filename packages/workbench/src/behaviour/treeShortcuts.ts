import type { ShortcutId } from '@shared/domain/shortcuts';
import type { Ref } from 'vue';
import { formatShortcut } from '../shortcuts/keys';
import { useShortcut } from '../shortcuts/useShortcut';

const TREE_IDS = [
  'tree.open',
  'tree.copyName',
  'tree.copyUri',
  'tree.rename',
  'tree.selectAll',
  'tree.duplicate',
  'tree.delete',
] as const satisfies readonly ShortcutId[];

export type TreeShortcutId = (typeof TREE_IDS)[number];

/** Acts on the row key the chord fired on. Omit an action a tree does not support. */
export type TreeShortcutActions = Partial<Record<TreeShortcutId, (key: string) => void>>;

/**
 * Binds the `tree.*` chords once for a list or tree built on `useListNavigation`. The row the
 * chord applies to is the focused row, else the roving stop. Chords typed in a text field are
 * left alone. `hints` holds the printed chord of each supported action, for menus and tooltips.
 */
export function useTreeShortcuts(
  nav: {
    container: Ref<HTMLElement | null | undefined>;
    activeKey: Ref<string | null>;
  },
  actions: TreeShortcutActions,
): { hints: Partial<Record<TreeShortcutId, string>> } {
  const supported = TREE_IDS.filter((id) => actions[id] !== undefined);
  useShortcut(
    nav.container as Ref<HTMLElement>,
    supported,
    (id, event) => {
      const focused = (event.target as HTMLElement | null)?.closest<HTMLElement>('[data-nav-key]');
      const key = focused?.dataset.navKey ?? nav.activeKey.value;
      if (key !== null && key !== undefined) actions[id as TreeShortcutId]?.(key);
    },
    { ignoreTextFields: true },
  );
  return {
    hints: Object.fromEntries(supported.map((id) => [id, formatShortcut(id)])),
  };
}
