import { defineAsyncComponent } from 'vue';

const load = () => import('./AdeNotesEditor.vue');

/** The editor component and `@tiptap/vue-3` load on first use. The entry chunk still carries ProseMirror
 *  core, StarterKit and the Markdown tokenizer: `AdeNoteBody` renders note HTML through them. */
export const AdeNotesEditor = defineAsyncComponent(load);

/** Warms the editor chunk from a pointer hover on the control that opens it. */
export function prefetchNotesEditor(): void {
  void load();
}
