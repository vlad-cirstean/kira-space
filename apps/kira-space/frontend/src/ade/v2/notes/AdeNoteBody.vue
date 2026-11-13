<script setup lang="ts">
import { generateHTML } from '@tiptap/core';
import { useEventListener } from '@vueuse/core';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { isPrimaryModifier } from '@workbench/shortcuts/keys';
import { computed, useTemplateRef } from 'vue';
import { control } from '../../../bridge/control';
import { parseMarkdown, parseWithLineBreaks } from './notesDoc';
import { notesExtensions } from './notesExtensions';
import { noteProse } from './prose';

// Read-only note body as static HTML: the same schema and prose styling as the editor, without an
// editor instance per note. Editing swaps in the async `AdeNotesEditor`.
const props = defineProps<{
  body: string;
  /** Single newlines stay line breaks: agent text is plain text, not Markdown paragraphs. */
  hardBreaks?: boolean;
  flush?: boolean;
}>();

const extensions = notesExtensions();

// `v-html` is safe here: the HTML is ProseMirror schema serialization, which escapes text, and the
// Link extension rewrites a disallowed href (such as `javascript:`) to an empty one.
const html = computed(() =>
  generateHTML(
    props.hardBreaks ? parseWithLineBreaks(props.body) : parseMarkdown(props.body),
    extensions,
  )
    // Task items render as plain checkboxes here; a read-only body never toggles them.
    .replaceAll('<input ', '<input disabled '),
);

const root = useTemplateRef<HTMLElement>('root');

// Delegated: the links live inside the injected HTML.
function onClick(event: MouseEvent): void {
  const link = (event.target as HTMLElement | null)?.closest('a');
  if (!link || !isPrimaryModifier(event)) return;
  event.preventDefault();
  const href = link.getAttribute('href');
  if (href) fireAndForget(control.openExternal(link.href), 'ade.notes');
}
useEventListener(root, 'click', onClick);
</script>

<template>
  <div class="flex flex-1 flex-col overflow-hidden rounded-kira-sm">
    <div
      class="flex-1 cursor-text select-text overflow-auto text-kira-md leading-normal text-fg"
      data-testid="ade-notes-editor"
    >
      <!-- eslint-disable-next-line vue/no-v-html -->
      <div
        ref="root"
        :class="[noteProse(flush), 'whitespace-pre-wrap break-words']"
        v-html="html"
      />
    </div>
  </div>
</template>
