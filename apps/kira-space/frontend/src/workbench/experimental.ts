import { type ComputedRef, computed, watch } from 'vue';
import { useModeStore } from '../state/mode';
import { useSettingsStore } from '../state/settings';
import { MODE_ORDER } from './modes';

/** Reads the saved flag, not the dialog draft: Save applies it live. */
export function useMemoryEnabled(): ComputedRef<boolean> {
  const settings = useSettingsStore();
  return computed(() => settings.experimental.memory);
}

export function useVisibleModes() {
  const enabled = useMemoryEnabled();
  return computed(() => MODE_ORDER.filter((m) => m !== 'memory' || enabled.value));
}

/** A window whose persisted mode is `memory` falls back to Space's default mode while the flag is off. */
export function installMemoryGate(): void {
  const enabled = useMemoryEnabled();
  const mode = useModeStore();
  watch(
    [enabled, () => mode.active],
    ([on, active]) => {
      if (!on && active === 'memory') mode.setMode('git');
    },
    { immediate: true },
  );
}
