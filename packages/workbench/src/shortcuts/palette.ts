import { defineStore } from 'pinia';
import { type MaybeRefOrGetter, onScopeDispose, shallowRef, toValue, watchEffect } from 'vue';

export interface PaletteCommand {
  id: string;
  label: string;
  run: () => void;
  /** Moves focus itself: runs after the dialog's close auto-focus so the restore cannot undo it. */
  movesFocus?: true;
}

export const usePaletteStore = defineStore('palette', () => {
  const commands = shallowRef<readonly PaletteCommand[]>([]);
  const open = shallowRef(false);

  function openPalette(): void {
    open.value = true;
  }
  function closePalette(): void {
    open.value = false;
  }
  function togglePalette(): void {
    open.value = !open.value;
  }
  function setCommands(next: readonly PaletteCommand[]): void {
    commands.value = next;
  }

  return { commands, open, openPalette, closePalette, togglePalette, setCommands };
});

/** Each app supplies its own command list once, from its root component's setup. */
export function providePaletteCommands(source: MaybeRefOrGetter<readonly PaletteCommand[]>): void {
  const store = usePaletteStore();
  watchEffect(() => store.setCommands(toValue(source)));
  onScopeDispose(() => store.setCommands([]));
}
