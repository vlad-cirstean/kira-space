// Before/after diff state for the cell editor; base is the stored value.
import { type ComputedRef, computed, type Ref, ref, watch } from 'vue';
import { cellKey, type SelectedCell } from '../../../state/cellSelection';
import { type ValueDiffSides, valueDiffSides } from './valueDiff';

export function useCellDiff(
  cell: () => SelectedCell,
  doc: Ref<string>,
  isDirty: ComputedRef<boolean>,
) {
  const open = ref(false);

  const base = computed<string | null>(() => {
    const c = cell();
    return c.original !== undefined ? c.original : c.value;
  });
  const available = computed(() =>
    base.value === null ? cell().value !== null || isDirty.value : doc.value !== base.value,
  );
  const shown = computed(() => open.value && available.value);
  const sides = computed<ValueDiffSides>(() =>
    valueDiffSides(base.value, doc.value, cell().column),
  );

  function close(): void {
    open.value = false;
  }
  function toggle(): void {
    open.value = !open.value;
  }

  watch(() => cellKey(cell()), close);

  return { available, shown, sides, toggle, close };
}
