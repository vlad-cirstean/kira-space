<script setup lang="ts">
import type { EditorLanguageId } from '@shared/domain/editor';
import { KIRA_EDITOR_THEME, loadMonaco } from '@workbench/editor/monaco';
import { monacoLanguageIdFor } from '@workbench/editor/monacoLanguages';
import { onMounted, onUnmounted, ref, watch } from 'vue';

type DiffEditor = import('monaco-editor').editor.IStandaloneDiffEditor;
type Monaco = Awaited<ReturnType<typeof loadMonaco>>;
type Model = import('monaco-editor').editor.ITextModel;

// Read-only Monaco diff of two texts. Inline below Monaco's width breakpoint. Callers size it with a wrapper, not a height class.
const props = withDefaults(
  defineProps<{ original: string; modified: string; language?: EditorLanguageId }>(),
  { language: 'plain' },
);

const root = ref<HTMLElement | null>(null);
const host = ref<HTMLElement | null>(null);
let diff: DiffEditor | null = null;
let mod: Monaco | null = null;
let originalModel: Model | null = null;
let modifiedModel: Model | null = null;

function syncDebugText(): void {
  if (!__KIRA_DEBUG_HOOKS__ || !root.value) return;
  root.value.setAttribute('data-kira-diff-original-text', props.original);
  root.value.setAttribute('data-kira-diff-modified-text', props.modified);
}

onMounted(async () => {
  const loaded = await loadMonaco();
  if (!host.value) return;
  mod = loaded;
  const languageId = monacoLanguageIdFor(props.language);
  originalModel = mod.editor.createModel(props.original, languageId);
  modifiedModel = mod.editor.createModel(props.modified, languageId);
  diff = mod.editor.createDiffEditor(host.value, {
    theme: KIRA_EDITOR_THEME,
    readOnly: true,
    domReadOnly: true,
    originalEditable: false,
    renderMarginRevertIcon: false,
    renderGutterMenu: false,
    automaticLayout: true,
    renderSideBySide: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    wordWrap: 'on',
    hideUnchangedRegions: { enabled: true },
  });
  diff.setModel({ original: originalModel, modified: modifiedModel });
  syncDebugText();
});

onUnmounted(() => {
  diff?.dispose();
  originalModel?.dispose();
  modifiedModel?.dispose();
  diff = null;
  mod = null;
});

watch(
  () => props.language,
  () => {
    const languageId = monacoLanguageIdFor(props.language);
    if (mod && originalModel) mod.editor.setModelLanguage(originalModel, languageId);
    if (mod && modifiedModel) mod.editor.setModelLanguage(modifiedModel, languageId);
  },
);

watch(
  () => [props.original, props.modified],
  () => {
    originalModel?.setValue(props.original);
    modifiedModel?.setValue(props.modified);
    syncDebugText();
  },
);
</script>

<template>
  <div ref="root" class="relative h-full min-h-40 overflow-hidden rounded-kira-sm">
    <div ref="host" class="absolute inset-0"></div>
  </div>
</template>
