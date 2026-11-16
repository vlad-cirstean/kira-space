import type { LayoutPatch } from '@shared/domain/layout';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { hydrateThenSubscribe } from '@workbench/state/hydrateThenSubscribe';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { control } from '../bridge/control';

// Space-only slice of the layout leaf set (`panel.ade.width`): the shared Layout schema carries
// only the panels both apps use.
interface AdeLayoutSlice {
  panel: { ade?: { width?: number } };
}

export const useAdeLayoutStore = defineStore('adeLayout', () => {
  /** 0 = list at 400px. */
  const width = ref(0);

  function apply(layout: unknown): void {
    width.value = (layout as AdeLayoutSlice).panel.ade?.width ?? 0;
  }

  const flush = useDebounceFn((w: number) => {
    fireAndForget(control.layoutSet({ panel: { ade: { width: w } } } as LayoutPatch), 'layout');
  }, TIMING.persist);

  function setWidth(w: number): void {
    width.value = w;
    void flush(w);
  }

  async function hydrate(): Promise<void> {
    await hydrateThenSubscribe({
      snapshot: () => control.layoutGetAll(),
      subscribe: (cb) => control.onLayoutChanged(cb),
      apply,
    });
  }

  return { width, setWidth, hydrate };
});
