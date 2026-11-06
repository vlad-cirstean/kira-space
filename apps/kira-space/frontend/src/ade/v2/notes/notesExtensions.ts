import { t } from '@theme/i18n';
import type { AnyExtension } from '@tiptap/core';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { Placeholder } from '@tiptap/extensions';
import { Markdown } from '@tiptap/markdown';
import StarterKit from '@tiptap/starter-kit';

// Shared by the Notes editor and the headless Markdown round-trip spec, so the two never drift.
// `underline: false`: Markdown has no underline, so saving would drop it. `link.openOnClick:
// false`: the editor opens links itself, through the OS.
export function notesExtensions(opts: { placeholder?: string } = {}): AnyExtension[] {
  return [
    StarterKit.configure({
      underline: false,
      link: { openOnClick: false, autolink: true },
    }),
    TaskList,
    TaskItem.configure({
      nested: true,
      a11y: {
        checkboxLabel: (node) =>
          node.textContent
            ? t('space.ade.notes.taskItem.aria', { text: node.textContent })
            : t('space.ade.notes.taskItemEmpty.aria'),
      },
    }),
    Placeholder.configure({
      placeholder: opts.placeholder ?? t('space.ade.notes.placeholder.placeholder'),
    }),
    Markdown,
  ];
}
