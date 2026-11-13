import { defineAsyncComponent } from 'vue';

const load = () => import('./AdeNotesEditor.vue');

/** The TipTap editor loads on first use, so the entry chunk carries no ProseMirror. */
export const AdeNotesEditor = defineAsyncComponent(load);

/** Warms the editor chunk from a pointer hover on the control that opens it. */
export function prefetchNotesEditor(): void {
  void load();
}
