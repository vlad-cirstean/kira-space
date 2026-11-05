<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import { type Chord, SHORTCUTS, type ShortcutId } from '@shared/domain/shortcuts';
import {
  KIRA_EDITOR_THEME,
  loadMonaco,
  type MonacoModule,
  overflowWidgetsContainer,
} from '@workbench/editor/monaco';
import { monacoLanguageIdFor } from '@workbench/editor/monacoLanguages';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';

type Editor = import('monaco-editor').editor.IStandaloneCodeEditor;
type Model = import('monaco-editor').editor.ITextModel;

// Monaco for a config file (YAML, Markdown, plain text). The save and submit chords bind inside
// the editor because Monaco consumes them before a dialog keydown handler sees them.
const props = defineProps<{
  language: EditorLanguageId;
  ariaLabel: string;
  problem?: { line: number; message: string } | null;
  readonly?: boolean;
  minLines?: number;
}>();
const emit = defineEmits<{ save: []; submit: [] }>();
const text = defineModel<string>({ required: true });

const root = ref<HTMLElement | null>(null);
const ready = ref(false);
let mod: MonacoModule | null = null;
let editor: Editor | null = null;
let model: Model | null = null;
let applyingExternal = false;

const minHeight = computed(() => (props.minLines ? `${props.minLines * 20 + 16}px` : undefined));

function syncDebugText(): void {
  if (!__KIRA_DEBUG_HOOKS__) return;
  root.value?.setAttribute('data-kira-editor-text', model?.getValue() ?? text.value);
}

function paintProblem(): void {
  if (!mod || !model) return;
  const target = model;
  const p = props.problem;
  const line = p ? Math.min(Math.max(1, p.line), target.getLineCount()) : 1;
  mod.editor.setModelMarkers(
    target,
    'kira',
    p
      ? [
          {
            startLineNumber: line,
            startColumn: 1,
            endLineNumber: line,
            endColumn: target.getLineMaxColumn(line),
            severity: mod.MarkerSeverity.Error,
            message: p.message,
          },
        ]
      : [],
  );
}

function keybindingFor(loaded: MonacoModule, id: ShortcutId): number {
  const chord: Chord = SHORTCUTS[id].chord;
  const key =
    chord.key === 'Return'
      ? loaded.KeyCode.Enter
      : loaded.KeyCode[`Key${chord.key.toUpperCase()}` as keyof typeof loaded.KeyCode];
  return (
    (chord.cmdOrCtrl ? loaded.KeyMod.CtrlCmd : 0) | (chord.shift ? loaded.KeyMod.Shift : 0) | key
  );
}

onMounted(async () => {
  const loaded = await loadMonaco();
  if (!root.value) return;
  mod = loaded;
  model = loaded.editor.createModel(text.value, monacoLanguageIdFor(props.language));
  model.updateOptions({ tabSize: 2, insertSpaces: true });
  editor = loaded.editor.create(root.value, {
    model,
    theme: KIRA_EDITOR_THEME,
    ariaLabel: props.ariaLabel,
    automaticLayout: true,
    readOnly: props.readonly,
    domReadOnly: props.readonly,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    wordWrap: 'on',
    folding: false,
    glyphMargin: false,
    renderLineHighlight: 'none',
    occurrencesHighlight: 'off',
    quickSuggestions: false,
    // Typed and pasted text lands as written: no indent guessing in a whitespace-significant file.
    autoIndent: 'none',
    fixedOverflowWidgets: true,
    overflowWidgetsDomNode: overflowWidgetsContainer(),
    padding: { top: 8, bottom: 8 },
  });
  const { KeyMod, KeyCode } = loaded;
  const target = model;
  // Select All, Undo and Redo fall through to a native textarea command in WebKit and do nothing
  // (MonacoHost.vue); run them on the model directly.
  editor.onKeyDown((e) => {
    if (e.equals(KeyMod.CtrlCmd | KeyCode.KeyA)) {
      editor?.setSelection(target.getFullModelRange());
    } else if (e.equals(KeyMod.CtrlCmd | KeyCode.KeyZ)) {
      void target.undo();
    } else if (
      e.equals(KeyMod.CtrlCmd | KeyMod.Shift | KeyCode.KeyZ) ||
      e.equals(KeyMod.CtrlCmd | KeyCode.KeyY)
    ) {
      void target.redo();
    } else {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
  });
  editor.addCommand(keybindingFor(loaded, 'form.save'), () => emit('save'));
  editor.addCommand(keybindingFor(loaded, 'form.submit'), () => emit('submit'));
  model.onDidChangeContent(() => {
    if (!applyingExternal && model) text.value = model.getValue();
    syncDebugText();
  });
  ready.value = true;
  paintProblem();
  syncDebugText();
});

onUnmounted(() => {
  editor?.dispose();
  model?.dispose();
  editor = null;
  model = null;
});

// An outside write (reload, a form edit) replaces the text as one undoable edit.
watch(text, (next) => {
  if (!model || model.getValue() === next) return;
  applyingExternal = true;
  model.pushStackElement();
  model.pushEditOperations(null, [{ range: model.getFullModelRange(), text: next }], () => null);
  model.pushStackElement();
  applyingExternal = false;
  syncDebugText();
});

watch(() => props.problem, paintProblem);
watch(
  () => props.readonly,
  (readOnly) => editor?.updateOptions({ readOnly, domReadOnly: readOnly }),
);
</script>

<template>
  <div
    ref="root"
    class="monaco-host relative h-full min-h-40 overflow-hidden rounded-kira-sm border border-border"
    :style="{ minHeight }"
    :data-language="language"
  >
    <pre
      v-if="!ready"
      class="m-0 h-full overflow-auto whitespace-pre-wrap bg-bg px-2 py-2 font-data text-kira-md text-fg"
    >{{ text }}</pre>
  </div>
</template>
