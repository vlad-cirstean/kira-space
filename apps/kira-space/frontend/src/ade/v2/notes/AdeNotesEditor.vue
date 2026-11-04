<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { EditorContent, useEditor } from '@tiptap/vue-3';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { isPrimaryModifier } from '@workbench/shortcuts/keys';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { control } from '../../../bridge/control';
import { notesExtensions } from './notesExtensions';

// Notes editor: TipTap + `@tiptap/markdown`. Loads and saves Markdown only; ProseMirror JSON never
// reaches the wire.
const props = defineProps<{
  notes: string;
  itemId: string;
  /** Archived task: no toolbar, no edits, no save. */
  readonly?: boolean;
}>();

const emit = defineEmits<{ save: [itemId: string, value: string] }>();

let lastEmitted = props.notes;
// The item the editor text currently belongs to; lags `props.itemId` until the switch watcher runs.
let contentItemId = props.itemId;

function flush(): void {
  const ed = editor.value;
  if (!ed || props.readonly) return;
  const md = ed.getMarkdown();
  if (md !== lastEmitted) {
    lastEmitted = md;
    emit('save', contentItemId, md);
  }
}

const debouncedFlush = useDebounceFn(flush, TIMING.autosave);

const PROSE = [
  'ade-notes-prose kira-prose min-h-full px-3 py-2 outline-none',
  '[&_p:last-child]:mb-0',
  "[&_ul[data-type='taskList']]:list-none [&_ul[data-type='taskList']]:pl-[0.2em]",
  "[&_ul[data-type='taskList']_li]:flex [&_ul[data-type='taskList']_li]:items-start [&_ul[data-type='taskList']_li]:gap-[0.4em]",
  "[&_input[type='checkbox']]:accent-ok",
  "[&_li[data-checked='true']>div]:text-subtle [&_li[data-checked='true']>div]:line-through",
  '[&_p.is-editor-empty:first-child]:before:pointer-events-none [&_p.is-editor-empty:first-child]:before:float-left [&_p.is-editor-empty:first-child]:before:h-0 [&_p.is-editor-empty:first-child]:before:text-subtle [&_p.is-editor-empty:first-child]:before:content-[attr(data-placeholder)]',
].join(' ');

const editor = useEditor({
  content: props.notes,
  contentType: 'markdown',
  extensions: notesExtensions(),
  editable: !props.readonly,
  editorProps: {
    attributes: { class: PROSE, 'aria-label': t('space.common.notes.label') },
    handleClick(_view, _pos, event) {
      const target = event.target as HTMLElement | null;
      const link = target?.closest('a');
      if (link && isPrimaryModifier(event)) {
        event.preventDefault();
        fireAndForget(control.openExternal(link.href), 'ade.notes');
        return true;
      }
      return false;
    },
  },
  onUpdate: () => {
    if (!props.readonly) debouncedFlush();
  },
  onBlur: () => flush(),
});

// Re-set content on an item switch (after flushing the outgoing item's pending edit), or when
// incoming `notes` differs from the last emit and the editor is unfocused; never mid-edit.
watch(
  () => props.itemId,
  () => {
    flush();
    contentItemId = props.itemId;
    const ed = editor.value;
    if (!ed) return;
    lastEmitted = props.notes;
    ed.commands.setContent(props.notes, { contentType: 'markdown', emitUpdate: false });
  },
);

watch(
  () => props.notes,
  (next) => {
    const ed = editor.value;
    if (!ed || ed.isFocused || next === lastEmitted) return;
    lastEmitted = next;
    ed.commands.setContent(next, { contentType: 'markdown', emitUpdate: false });
  },
);

onBeforeUnmount(() => {
  flush();
  editor.value?.destroy();
});

// Toolbar: codicon where the theme has one, else the glyph text.
interface ToolbarButton {
  label: string;
  icon: string;
  glyph: string;
  active: boolean;
  run: () => void;
}

const toolbar = computed<ToolbarButton[]>(() => {
  const ed = editor.value;
  if (!ed) return [];
  const chain = () => ed.chain().focus();
  return [
    {
      label: t('space.ade.notes.bold.tooltip'),
      icon: 'bold',
      glyph: 'B',
      active: ed.isActive('bold'),
      run: () => chain().toggleBold().run(),
    },
    {
      label: t('space.ade.notes.italic.tooltip'),
      icon: 'italic',
      glyph: 'I',
      active: ed.isActive('italic'),
      run: () => chain().toggleItalic().run(),
    },
    {
      label: t('common.code.label'),
      icon: 'code',
      glyph: '</>',
      active: ed.isActive('code'),
      run: () => chain().toggleCode().run(),
    },
    {
      label: t('space.ade.notes.heading.tooltip'),
      icon: '',
      glyph: 'H',
      active: ed.isActive('heading', { level: 2 }),
      run: () => chain().toggleHeading({ level: 2 }).run(),
    },
    {
      label: t('space.ade.notes.bulletedList.tooltip'),
      icon: 'list-unordered',
      glyph: '•',
      active: ed.isActive('bulletList'),
      run: () => chain().toggleBulletList().run(),
    },
    {
      label: t('space.ade.notes.numberedList.tooltip'),
      icon: 'list-ordered',
      glyph: '1.',
      active: ed.isActive('orderedList'),
      run: () => chain().toggleOrderedList().run(),
    },
    {
      label: t('space.ade.notes.checklist.tooltip'),
      icon: 'checklist',
      glyph: '☐',
      active: ed.isActive('taskList'),
      run: () => chain().toggleTaskList().run(),
    },
    {
      label: t('space.ade.notes.link.tooltip'),
      icon: 'link',
      glyph: '🔗',
      active: ed.isActive('link'),
      run: openLink,
    },
  ];
});

const linkOpen = ref(false);
const linkUrl = ref('');

function openLink(): void {
  linkUrl.value = (editor.value?.getAttributes('link').href as string | undefined) ?? '';
  linkOpen.value = true;
}

function applyLink(): void {
  const url = linkUrl.value.trim();
  linkOpen.value = false;
  if (!/^https?:\/\//.test(url)) return;
  const ed = editor.value;
  if (!ed) return;
  if (ed.state.selection.empty) {
    ed.chain()
      .focus()
      .insertContent({ type: 'text', text: url, marks: [{ type: 'link', attrs: { href: url } }] })
      .run();
  } else {
    ed.chain().focus().setLink({ href: url }).run();
  }
}

function onLinkKey(e: KeyboardEvent): void {
  if (e.key === 'Enter') {
    e.preventDefault();
    applyLink();
  } else if (e.key === 'Escape') {
    linkOpen.value = false;
  }
}

function keepFocus(e: MouseEvent): void {
  e.preventDefault();
}
</script>

<template>
  <div
    class="flex min-h-35 flex-1 flex-col overflow-hidden rounded-kira-sm border border-border bg-bg"
  >
    <div
      v-if="!readonly"
      role="toolbar"
      :aria-label="t('space.ade.notes.formatting.aria')"
      class="flex shrink-0 items-center gap-0.5 border-b border-border bg-chrome px-1.5 py-1"
    >
      <span class="px-1.5 text-kira-sm text-muted-foreground">{{
        t('space.common.notes.label')
      }}</span>
      <Button
        v-for="btn in toolbar"
        :key="btn.label"
        variant="toolbar"
        size="kira-icon"
        class="text-kira-sm"
        :aria-label="btn.label"
        :aria-pressed="btn.active"
        :title="btn.label"
        data-testid="ade-notes-toolbar-btn"
        @mousedown="keepFocus"
        @click="btn.run"
      >
        <CodiconIcon v-if="btn.icon" :name="btn.icon" :size="12" />
        <span v-else>{{ btn.glyph }}</span>
      </Button>
      <template v-if="linkOpen">
        <Label for="ade-notes-link" class="sr-only">{{ t('space.ade.notes.linkUrl.label') }}</Label>
        <Input
          id="ade-notes-link"
          v-model="linkUrl"
          :placeholder="t('space.ade.notes.linkUrl.example')"
          size="kira"
          class="ml-1 w-42.5 font-data"
          @keydown="onLinkKey"
        />
        <Button
          variant="dialog"
          size="kira"
          class="shrink-0"
          @mousedown="keepFocus"
          @click="applyLink"
          >{{
            t('common.action.add.label')
          }}</Button
        >
      </template>
    </div>
    <EditorContent
      :editor="editor"
      class="min-h-25 flex-1 overflow-auto text-kira-md leading-normal text-fg focus-within:focus-ring"
      data-testid="ade-notes-editor"
    />
  </div>
</template>
