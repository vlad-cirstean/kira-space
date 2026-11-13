<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { t } from '@theme/i18n';
import { Editor, EditorContent } from '@tiptap/vue-3';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { isPrimaryModifier } from '@workbench/shortcuts/keys';
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue';
import { control } from '../../../bridge/control';
import { parseWithLineBreaks } from './notesDoc';
import { notesExtensions } from './notesExtensions';
import { noteProse } from './prose';

// Notes editor: TipTap + `@tiptap/markdown`. Takes and emits Markdown only; ProseMirror JSON never
// reaches the wire. It emits only after a user edit: Markdown parse and serialize normalizes text,
// so an untouched editor must not report a changed string.
const props = defineProps<{
  /** No toolbar, no edits. */
  readonly?: boolean;
  /** Single newlines stay line breaks: agent text is plain text, not Markdown paragraphs. Fixed at mount. */
  hardBreaks?: boolean;
  /** Aria label and toolbar caption. */
  label?: string;
  placeholder?: string;
  /** Smaller minimum height, for popovers and dialogs. */
  compact?: boolean;
  /** Focus the end of the text when it mounts. */
  focusOnMount?: boolean;
  /** No inner padding: text starts at the surrounding content edge. Fixed at mount. */
  flush?: boolean;
}>();

const model = defineModel<string>({ required: true });
const emit = defineEmits<{ submit: [] }>();

const caption = computed(() => props.label ?? t('space.common.notes.label'));
let lastEmitted = model.value;
let dirty = false;

function emitPending(): void {
  const ed = editor.value;
  if (props.readonly || !dirty) return;
  dirty = false;
  const md = ed.getMarkdown();
  if (md !== lastEmitted) {
    lastEmitted = md;
    model.value = md;
  }
}

const debouncedFlush = useDebounceFn(emitPending, TIMING.autosave);

const PROSE = noteProse(props.flush);

// Built in setup, not on mount: the text is in the first frame.
const editor = shallowRef(
  new Editor({
    ...(props.hardBreaks
      ? { content: parseWithLineBreaks(model.value) }
      : { content: model.value, contentType: 'markdown' as const }),
    extensions: notesExtensions({ placeholder: props.placeholder }),
    editable: !props.readonly,
    autofocus: props.focusOnMount ? 'end' : false,
    editorProps: {
      attributes: { class: PROSE, 'aria-label': caption.value },
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
      handleKeyDown(_view, event) {
        if (props.readonly || event.key !== 'Enter' || !isPrimaryModifier(event)) return false;
        event.preventDefault();
        emitPending();
        emit('submit');
        return true;
      },
    },
    onUpdate: () => {
      if (props.readonly) return;
      dirty = true;
      debouncedFlush();
    },
    onBlur: () => emitPending(),
  }),
);

// Outside changes (a prefill that loads late, a saved body) replace the text, never mid-edit: a
// focused editor keeps its text unless it is still empty.
watch(model, (next) => {
  const ed = editor.value;
  if (next === lastEmitted || (ed.isFocused && !props.readonly && !ed.isEmpty)) return;
  lastEmitted = next;
  dirty = false;
  ed.commands.setContent(next, { contentType: 'markdown', emitUpdate: false });
});

onBeforeUnmount(() => {
  emitPending();
  editor.value.destroy();
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
  linkUrl.value = (editor.value.getAttributes('link').href as string | undefined) ?? '';
  linkOpen.value = true;
}

function applyLink(): void {
  const url = linkUrl.value.trim();
  linkOpen.value = false;
  if (!/^https?:\/\//.test(url)) return;
  const ed = editor.value;
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
    class="flex flex-1 flex-col overflow-hidden rounded-kira-sm"
    :class="readonly ? '' : ['border border-border bg-bg', compact ? 'min-h-24' : 'min-h-35']"
  >
    <div
      v-if="!readonly"
      role="toolbar"
      :aria-label="t('space.ade.notes.formatting.aria')"
      class="flex shrink-0 items-center gap-0.5 bg-chrome px-1.5 py-1"
    >
      <span class="px-1.5 text-kira-sm text-muted-foreground">{{ caption }}</span>
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
      class="flex-1 overflow-auto text-kira-md leading-normal text-fg focus-within:focus-ring"
      :class="readonly ? 'cursor-text select-text' : compact ? 'min-h-14' : 'min-h-25'"
      data-testid="ade-notes-editor"
    />
  </div>
</template>
