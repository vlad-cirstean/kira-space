import { revealElement } from '@workbench/behaviour/focus';
import { nextTick, type Ref, watch } from 'vue';

interface RevealInput {
  collapsedDirs: Ref<Set<string>>;
  rows: Readonly<Ref<readonly { kind: string; node: { path: string } }[]>>;
  visibleRows: Readonly<Ref<readonly unknown[]>>;
  showMore: () => void;
  treeEl: Readonly<Ref<HTMLElement | null>>;
}

/** Brings the selected file's row into view however the selection moved: expands its collapsed
 *  parents, raises the render cap past it, scrolls the nearest edge. Keyed on the path, so a list
 *  refresh that only shifts indexes never re-expands a folder the user collapsed. A row the filter
 *  hides stays hidden. Never moves focus. */
export function useRevealSelectedFile(
  path: Readonly<Ref<string | null>>,
  input: RevealInput,
): void {
  watch(path, async (selected) => {
    if (selected === null) return;
    const dirs = [...input.collapsedDirs.value].filter((dir) => !selected.startsWith(`${dir}/`));
    if (dirs.length !== input.collapsedDirs.value.size) input.collapsedDirs.value = new Set(dirs);
    const at = input.rows.value.findIndex((r) => r.kind === 'file' && r.node.path === selected);
    if (at < 0) return;
    while (at >= input.visibleRows.value.length) input.showMore();
    await nextTick();
    const key = `file:${selected}`;
    const row = [...(input.treeEl.value?.querySelectorAll('[data-nav-key]') ?? [])].find(
      (el) => el.getAttribute('data-nav-key') === key,
    );
    if (row) revealElement(row);
  });
}
