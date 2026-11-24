import { type Ref, watch } from 'vue';

interface RevealInput {
  collapsedDirs: Ref<Set<string>>;
  rows: Readonly<Ref<readonly { kind: string; node: { path: string } }[]>>;
  /** Brings the row with this key into view (the virtualizer's `revealKey`, or the capped list's raise-then-scroll). */
  reveal: (key: string) => void | Promise<void>;
}

/** Brings the selected file's row into view however the selection moved: expands its collapsed
 *  parents, then reveals the row. Keyed on the path, so a list refresh that only shifts indexes
 *  never re-expands a folder the user collapsed. A row the filter hides stays hidden. Never moves
 *  focus. */
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
    await input.reveal(`file:${selected}`);
  });
}
