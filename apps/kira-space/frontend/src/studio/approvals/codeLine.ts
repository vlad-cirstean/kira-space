import { computed } from 'vue';
import { useSettingsStore } from '../../state/settings';

/** The data font's line box in px: the flush editor's 1.5 multiplier, also `leading-normal`. */
export function useCodeLineHeight() {
  const settings = useSettingsStore();
  return computed(() => Math.round(1.5 * settings.appearance.fontSize));
}
