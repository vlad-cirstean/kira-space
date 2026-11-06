<script setup lang="ts">
// A drop-in for `CodeMirrorHost.vue` — same prop names, same emits, same exposed
// methods, so every mount site changes only its import and its tag name. Two prop *types* change
// because their old types were CodeMirror types; everything else here is deliberately the
// same shape as `CodeMirrorHost.vue`, restated for Monaco's own API rather than redesigned.

import type { EditorLanguageId } from '@shared/domain/editor';
import { useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import {
  KIRA_EDITOR_THEME,
  loadMonaco,
  type MonacoModule,
  overflowWidgetsContainer,
} from '@workbench/editor/monaco';
import { monacoLanguageIdFor } from '@workbench/editor/monacoLanguages';
import type { RangeHighlight } from '@workbench/editor/ranges';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useSettingsStore } from '../state/settings';
import type { SqlDialect } from '../views/shared/sqlIdent';
import { pumpChunks, textChunks } from './chunkedText';
import type {
  EditorAsyncCompletionSource,
  EditorCompletion,
  EditorCompletionKind,
  EditorCompletionResult,
  EditorCompletionSource,
} from './completion';
import { ensureCustomLanguage } from './customLanguages';
import type { ConsoleDiagnostic } from './diagnostics';
import { type ConsoleHoverInfo, escapeMarkdownSyntaxTokens, fenceMarkdownValue } from './hoverInfo';

// `import('monaco-editor').X` inline type references, not a static `import type {...}` — this
// ambient module's own established style for its types.
type StandaloneCodeEditor = import('monaco-editor').editor.IStandaloneCodeEditor;
type TextModel = import('monaco-editor').editor.ITextModel;
type DecorationsCollection = import('monaco-editor').editor.IEditorDecorationsCollection;
type ConstructionOptions = import('monaco-editor').editor.IStandaloneEditorConstructionOptions;
type MonacoDisposable = import('monaco-editor').IDisposable;
type CompletionItemProvider = import('monaco-editor').languages.CompletionItemProvider;
type HoverProvider = import('monaco-editor').languages.HoverProvider;
type CompletionItemKind = import('monaco-editor').languages.CompletionItemKind;
type MonacoRange = import('monaco-editor').IRange;
type MarkdownString = import('monaco-editor').IMarkdownString;
type DeltaDecoration = import('monaco-editor').editor.IModelDeltaDecoration;
type MarkerData = import('monaco-editor').editor.IMarkerData;

const props = defineProps<{
  language: EditorLanguageId;
  /** Only consulted when `language === 'sql'` — kept for prop-shape parity with
   *  `CodeMirrorHost.vue`; Monaco's own `sql` Monarch has no per-dialect keyword set (P60b's own
   *  scope owns SQL language services proper). */
  sqlDialect?: SqlDialect;
  readonly?: boolean;
  autocomplete?: boolean;
  /** §3.1: pure data in, pure data out — replaces `@codemirror/autocomplete`'s `CompletionSource`
   *  at this seam. */
  completionSources?: readonly EditorCompletionSource[];
  /** A grammar this host registers itself (once per id) in place of the `language` mapping. */
  monacoLanguage?: { id: string; register(m: MonacoModule): void };
  /** Overrides the default `.`, `$`, `{` for this host's provider. */
  triggerCharacters?: readonly string[];
  /** Server-backed sources, awaited alongside the sync ones. */
  asyncCompletionSources?: readonly EditorAsyncCompletionSource[];
  lintSource?: (doc: string) => ConsoleDiagnostic[];
  /** §3.1: simpler than the CodeMirror shape it replaces — `hover.ts`'s own `buildHoverSource`
   *  existed only to turn a pure lookup into a CodeMirror object; that glue collapses into the
   *  pure lookup itself here. */
  hoverSource?: (doc: string, offset: number) => ConsoleHoverInfo | null;
  singleLine?: boolean;
  /** The save chord emits `save`; Monaco consumes it before the view handler otherwise. */
  saveCommand?: boolean;
  /** Wraps regardless of the appearance setting. */
  forceWrap?: boolean;
  rangeHighlights?: (doc: string) => readonly RangeHighlight[];
  autoCloseBrackets?: boolean;
  keepSelectionOnExternalSync?: boolean;
  /** Restored once the first content lands (the proof window's saved query scroll). */
  scrollTop?: number;
  /** Caret offset restored with the first content, before `scrollTop`; never scrolls to it. */
  cursor?: number;
}>();

const docModel = defineModel<string>('doc', { required: true });
const emit = defineEmits<{
  'update:cursor': [pos: number];
  'update:scrollTop': [top: number];
  'update:contentHeight': [height: number];
  /** The first content is in and `scrollTop` is applied and painted. */
  ready: [];
  /** Enter in a `singleLine` host with no suggestion open. */
  submit: [];
  save: [];
}>();

const settingsStore = useSettingsStore();

const rootRef = ref<HTMLElement | null>(null);
// §3.3: renders the raw doc as text while loadMonaco()'s import is in flight — never an empty box.
// The pending <pre> sits inside `rootRef`, which `editor.create` measures synchronously: a
// full-doc <pre> forced a layout of megabytes of wrapped text there. Cap it.
const PENDING_PREVIEW_CHARS = 16_384;
// Above this, yield one painted frame before Monaco work so the capped preview shows first.
const LARGE_DOC_CHARS = 262_144;
// Read-only docs over LARGE_DOC_CHARS fill in chunks of this size, a frame apart: one `setValue` of
// a multi-MB doc blocked the main thread for over a second (measured, P160).
const FILL_CHUNK_CHARS = 262_144;

const previewShown = ref(true);
const previewDoc = computed(() =>
  docModel.value.length > PENDING_PREVIEW_CHARS
    ? docModel.value.slice(0, PENDING_PREVIEW_CHARS)
    : docModel.value,
);
let fillCtrl: AbortController | null = null;
// §4.7's own `externalSync` annotation equivalent — a plain flag, no annotation machinery needed.
let applyingExternal = false;
let filling = false;
const cancelFill = (): void => {
  if (!fillCtrl) return;
  fillCtrl.abort();
  fillCtrl = null;
  filling = false;
  applyingExternal = false;
};
let lastAppliedDoc: string | null = null;
let lastAppliedVersionId = -1;

// §3.2: never a ref/shallowRef/reactive — same no-reactivity rule as CodeMirrorHost.vue's own
// `view` (D4), restated here for the same reason: Vue must not proxy Monaco's internals on every
// keystroke.
let mod: MonacoModule | null = null;
let editor: StandaloneCodeEditor | null = null;
let model: TextModel | null = null;
let decorations: DecorationsCollection | null = null;
let completionDisposable: MonacoDisposable | null = null;
let hoverDisposable: MonacoDisposable | null = null;
let wrapDisposable: MonacoDisposable | null = null;
// Never shown: the context menu is off, so the id doubles as Monaco's required label.
const SUBMIT_ACTION = 'kira.submit';
const SAVE_ACTION = 'kira.save';
let actionDisposables: MonacoDisposable[] = [];

function resolveWordWrap(): 'on' | 'off' {
  if (props.singleLine) return 'off';
  if (props.forceWrap) return 'on';
  return settingsStore.appearance.wordWrap ? 'on' : 'off';
}

// `wrapSelection.ts`'s own selection-wrap handler, ported to Monaco's `onKeyDown` rather
// than relying on Monaco's own `autoSurround` — `autoSurround` has no carve-out for a
// whole-document selection, and this app's own `mongo.spec.ts` depends on Select-All-then-retype
// staying a plain replace. A self-contained port guarantees the exact rule regardless of Monaco's
// internal behaviour, so `autoSurround` itself stays 'never' below and this handler is the only
// source of wrap-on-type. Deliberately unconditional (fires on every host, not gated by
// `autoCloseBrackets`) — wrapping a *non-empty* selection is never the thing that prop's own
// empty-pair auto-close would have silently "fixed" (`wrapSelection.ts`'s own reasoning).
const WRAP_PAIRS: Record<string, string> = {
  '(': ')',
  '[': ']',
  '{': '}',
  "'": "'",
  '"': '"',
  '`': '`',
};

// Select All, Undo and Redo are Monaco's only three core commands built on
// `EditorOrNativeTextInputCommand` (`coreCommands.js`, verified against the pinned 0.56.0) — its
// "editor has focus" branch checks `codeEditorService.getFocusedCodeEditor()?.hasTextFocus()`,
// which comes back false in WebKit even though the `.inputarea` textarea genuinely holds DOM
// focus (root cause not chased further: `view.isFocused()`'s own internal tracking, decoupled from
// `document.activeElement`, apparently lags a same-tick DOM focus + keypress here). All three then
// fall through to a "generic dom input" branch that runs a native `execCommand` on the hidden
// textarea instead of the real command — a plain no-op for Undo/Redo (the textarea has no
// undo/redo history of its own to speak of) and a same-DOM-node-only select for Select All. Each
// is handled directly here instead, calling the model's own API and bypassing that lookup
// entirely — this is the same code Monaco's own "editor has focus" branch would have run.
function attachWrapOnType(ed: StandaloneCodeEditor, m: TextModel): MonacoDisposable {
  return ed.onKeyDown((e) => {
    // `.toLowerCase()`, not a literal character: a real keypress always reports a lowercase
    // `key` when unshifted, but `page.keyboard.press('Control+A')` (several specs' own
    // capitalised `SELECT_ALL` constant) makes Playwright synthesize the browser event with the
    // literal key name it was given, `key: 'A'`, capital and shiftless both.
    const lowerKey = e.browserEvent.key.toLowerCase();
    if ((e.ctrlKey || e.metaKey) && !e.altKey) {
      if (!e.shiftKey && lowerKey === 'a') {
        e.preventDefault();
        e.stopPropagation();
        ed.setSelection(m.getFullModelRange());
        return;
      }
      if (lowerKey === 'z') {
        e.preventDefault();
        e.stopPropagation();
        void (e.shiftKey ? m.redo() : m.undo());
        return;
      }
      // Ctrl+Y (not Cmd+Y, which is a no-op here): the Windows/Linux-only alternate redo chord.
      if (e.ctrlKey && !e.metaKey && !e.shiftKey && lowerKey === 'y') {
        e.preventDefault();
        e.stopPropagation();
        void m.redo();
        return;
      }
    }
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    const key = e.browserEvent.key;
    const close = WRAP_PAIRS[key];
    if (!close) return;
    const sel = ed.getSelection();
    if (!sel || sel.isEmpty()) return;
    const from = m.getOffsetAt(sel.getStartPosition());
    const to = m.getOffsetAt(sel.getEndPosition());
    // A whole-document selection (Select All, then retype) is a "replace everything" click, not a
    // "wrap everything" one — see `wrapSelection.ts:37`'s identical carve-out.
    if (from === 0 && to === m.getValue().length) return;
    e.preventDefault();
    e.stopPropagation();
    const selected = m.getValueInRange(sel);
    ed.executeEdits('kira-wrap-selection', [{ range: sel, text: `${key}${selected}${close}` }]);
    const newFrom = m.getPositionAt(from + key.length);
    const newTo = m.getPositionAt(to + key.length);
    ed.setSelection({
      startLineNumber: newFrom.lineNumber,
      startColumn: newFrom.column,
      endLineNumber: newTo.lineNumber,
      endColumn: newTo.column,
    });
  });
}

function kindFor(m: MonacoModule, type?: EditorCompletionKind): CompletionItemKind {
  const K = m.languages.CompletionItemKind;
  switch (type) {
    case 'variable':
      return K.Variable;
    case 'method':
      return K.Method;
    case 'function':
      return K.Function;
    case 'class':
      return K.Class;
    case 'keyword':
      return K.Keyword;
    case 'property':
      return K.Property;
    default:
      return K.Text;
  }
}

function languageId(m: MonacoModule): string {
  const custom = props.monacoLanguage;
  if (!custom) return monacoLanguageIdFor(props.language);
  ensureCustomLanguage(m, custom);
  return custom.id;
}

function hasCompletion(): boolean {
  return Boolean(props.completionSources?.length || props.asyncCompletionSources?.length);
}

// Word-based suggestions stay off whenever completion is this host's own: with `autocomplete` on
// and no source (a cold SQL console), Monaco's default popped a stale-word suggestion mid-typing.
function wordBasedSuggestions(): 'off' | undefined {
  return props.autocomplete || hasCompletion() ? 'off' : undefined;
}

// §4.8: registered per host instance, model-scoped (returning empty suggestions for any other
// model) — Monaco's provider registry is global per language id, so an unscoped registration would
// leak one pane's completions into every other pane of the same language.
function buildCompletionProvider(m: MonacoModule): CompletionItemProvider {
  return {
    // Without trigger characters Monaco re-invokes a provider only as a word continues, never right
    // after `.`, `$` or `{` — the positions this app's sources key off. A host can override them.
    triggerCharacters: [...(props.triggerCharacters ?? ['.', '$', '{'])],
    async provideCompletionItems(candidateModel, position, context, token) {
      if (candidateModel !== model || !hasCompletion()) return { suggestions: [] };
      const doc = candidateModel.getValue();
      const offset = candidateModel.getOffsetAt(position);
      // Ctrl+Space is `Invoke`; sources gating an empty-word position on `explicit` need it.
      const explicit = context.triggerKind === m.languages.CompletionTriggerKind.Invoke;
      const ctx = { doc, offset, explicit };

      const toSuggestions = (result: EditorCompletionResult) => {
        const start = candidateModel.getPositionAt(result.from);
        const range: MonacoRange = {
          startLineNumber: start.lineNumber,
          startColumn: start.column,
          endLineNumber: position.lineNumber,
          endColumn: position.column,
        };
        return result.options.map((opt: EditorCompletion, index: number) => ({
          label: opt.label,
          kind: kindFor(m, opt.type),
          detail: opt.detail,
          // A source's `snippet` already uses Monaco's `$0` syntax; only the rule is set here.
          insertText: opt.snippet ?? opt.insert ?? opt.label,
          insertTextRules: opt.snippet
            ? m.languages.CompletionItemInsertTextRule.InsertAsSnippet
            : undefined,
          // Boosted entries sort first, stable among equals via the zero-padded index.
          sortText: `${opt.boost ? 0 : 1}${String(index).padStart(4, '0')}`,
          range,
        }));
      };

      for (const source of props.completionSources ?? []) {
        const result = source(ctx);
        if (result) return { suggestions: toSuggestions(result), incomplete: result.incomplete };
      }

      const asyncSources = props.asyncCompletionSources ?? [];
      if (asyncSources.length === 0) return { suggestions: [] };
      const abort = new AbortController();
      const cancel = token.onCancellationRequested(() => abort.abort());
      try {
        for (const source of asyncSources) {
          const result = await source(ctx, abort.signal);
          if (abort.signal.aborted) return { suggestions: [] };
          if (result) return { suggestions: toSuggestions(result), incomplete: result.incomplete };
        }
      } finally {
        cancel.dispose();
      }
      return { suggestions: [] };
    },
  };
}

// §4.4: registered per host instance, model-scoped the same way completion is.
function buildHoverProvider(): HoverProvider {
  return {
    provideHover(candidateModel, position) {
      if (candidateModel !== model || !props.hoverSource) return null;
      const offset = candidateModel.getOffsetAt(position);
      const info = props.hoverSource(candidateModel.getValue(), offset);
      if (!info) return null;
      const start = candidateModel.getPositionAt(info.from);
      const end = candidateModel.getPositionAt(info.to);
      const contents: MarkdownString[] = [];
      // §4.4/P108 Part 11 F8: `value` -> one fenced block, so a pretty-printed value keeps its line
      // breaks; the fence length itself is content-aware (fenceMarkdownValue) so a value containing
      // its own triple backtick can't close the fence early and spill the rest as Markdown.
      if (info.value !== undefined) {
        contents.push({ value: fenceMarkdownValue(info.value) });
      }
      // `supportHtml: false` blocks raw HTML only — Markdown syntax inside `line` (a table/
      // column name, a COMMENT ON COLUMN description, …) still rendered: a `[text](url)` comment
      // became a clickable link, `_x_`/`__init__` names rendered as italic/bold. Escaped so `line`
      // always renders as the literal text it is.
      for (const line of info.lines) {
        contents.push({ value: escapeMarkdownSyntaxTokens(line), supportHtml: false });
      }
      return {
        range: {
          startLineNumber: start.lineNumber,
          startColumn: start.column,
          endLineNumber: end.lineNumber,
          endColumn: end.column,
        },
        contents,
      };
    },
  };
}

function registerProviders(m: MonacoModule, id: string): void {
  completionDisposable?.dispose();
  completionDisposable = null;
  hoverDisposable?.dispose();
  hoverDisposable = null;
  if (hasCompletion()) {
    completionDisposable = m.languages.registerCompletionItemProvider(
      id,
      buildCompletionProvider(m),
    );
  }
  if (props.hoverSource) {
    hoverDisposable = m.languages.registerHoverProvider(id, buildHoverProvider());
  }
}

// §4.3: `variableHighlight.ts`'s `RangeHighlight` kept verbatim; the `ViewPlugin` becomes a
// decorations collection rebuilt on the same two triggers CodeMirror used — content change and a
// `rangeHighlights` prop-identity change.
function repaintRanges(): void {
  if (!decorations) return;
  if (!model || !props.rangeHighlights) {
    decorations.set([]);
    return;
  }
  const text = model.getValue();
  const built: DeltaDecoration[] = [];
  for (const r of props.rangeHighlights(text)) {
    // The same offset-validity filter `variableHighlight.ts:28` keeps — a caller's ranges can be
    // one keystroke stale.
    if (!(r.from >= 0 && r.from < r.to && r.to <= text.length)) continue;
    const start = model.getPositionAt(r.from);
    const end = model.getPositionAt(r.to);
    built.push({
      range: {
        startLineNumber: start.lineNumber,
        startColumn: start.column,
        endLineNumber: end.lineNumber,
        endColumn: end.column,
      },
      options: { inlineClassName: r.class, inlineClassNameAffectsLetterSpacing: false },
    });
  }
  decorations.set(built);
}

// §4.5: debounced by TIMING.preview, mapped to `editor.setModelMarkers` — a themed wavy underline plus a
// hover, no gutter and no lint panel (D24/D25's own reasoning, restated for Monaco).
const runLint = useDebounceFn(
  (
    m: MonacoModule,
    targetModel: TextModel,
    source: (text: string) => readonly ConsoleDiagnostic[],
  ) => {
    if (targetModel.isDisposed()) return;
    const markers: MarkerData[] = source(targetModel.getValue()).map((d) => {
      const start = targetModel.getPositionAt(d.from);
      const end = targetModel.getPositionAt(d.to);
      return {
        startLineNumber: start.lineNumber,
        startColumn: start.column,
        endLineNumber: end.lineNumber,
        endColumn: end.column,
        severity: d.severity === 'error' ? m.MarkerSeverity.Error : m.MarkerSeverity.Warning,
        message: d.message,
      };
    });
    m.editor.setModelMarkers(targetModel, 'kira', markers);
  },
  TIMING.preview,
);

function scheduleLint(m: MonacoModule): void {
  if (!model || !props.lintSource) {
    runLint.cancel();
    if (model) m.editor.setModelMarkers(model, 'kira', []);
    return;
  }
  void runLint(m, model, props.lintSource);
}

function applyBaseOptions(): ConstructionOptions {
  return {
    automaticLayout: true,
    readOnly: props.readonly,
    domReadOnly: props.readonly,
    lineNumbers: props.singleLine ? 'off' : 'on',
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    renderLineHighlight: 'none',
    folding: false,
    glyphMargin: false,
    fixedOverflowWidgets: true,
    // `fixedOverflowWidgets` alone keeps a widget a DOM descendant of this host
    // (only escaping ancestor `overflow: hidden` visually, via `position: fixed`) — this is what
    // reparents it to `document.body`, matching capability 20's own requirement and
    // `CodeMirrorHost.vue`'s old `tooltips({ parent: document.body })`.
    overflowWidgetsDomNode: overflowWidgetsContainer(),
    wordWrap: resolveWordWrap(),
    autoSurround: 'never', // §4.6: this host's own onKeyDown handler owns wrap-on-type instead.
    autoClosingBrackets: props.autoCloseBrackets ? 'languageDefined' : 'never',
    autoClosingQuotes: props.autoCloseBrackets ? 'languageDefined' : 'never',
    quickSuggestions: Boolean(props.autocomplete),
    quickSuggestionsDelay: 0,
    suggestOnTriggerCharacters: Boolean(props.autocomplete),
    acceptSuggestionOnEnter: 'off',
    tabCompletion: 'on',
    wordBasedSuggestions: wordBasedSuggestions(),
    occurrencesHighlight: 'off',
    hover: { delay: TIMING.hoverInfo, above: true },
    fontFamily: settingsStore.appearance.fontFamily,
    fontSize: settingsStore.appearance.fontSize,
    ...(props.singleLine
      ? { lineDecorationsWidth: 0, lineNumbersMinChars: 0, padding: { top: 0, bottom: 0 } }
      : { padding: { top: 8, bottom: 8 } }),
  };
}

function finishFill(m: TextModel, doc: string): void {
  filling = false;
  lastAppliedDoc = doc;
  lastAppliedVersionId = m.getVersionId();
  repaintRanges();
  if (mod) scheduleLint(mod);
  updateDebugHook();
}

// Replaces the model's content with `doc`. Read-only large docs stream in line-aligned chunks (no
// undo entries: `applyEdits`), yielding a frame between them; a newer fill or unmount cancels.
async function fillModel(doc: string, onFirstContent?: () => void): Promise<void> {
  const target = model;
  if (!target) return;
  cancelFill();
  const ctrl = new AbortController();
  fillCtrl = ctrl;
  filling = true;
  applyingExternal = true;
  lastAppliedVersionId = -1;
  try {
    if (!props.readonly || doc.length <= LARGE_DOC_CHARS) {
      target.setValue(doc);
      onFirstContent?.();
      finishFill(target, doc);
      return;
    }
    target.setValue('');
    const done = await pumpChunks(
      textChunks(doc, { chunkChars: FILL_CHUNK_CHARS, lineAligned: true }),
      (chunk) => {
        const line = target.getLineCount();
        const col = target.getLineMaxColumn(line);
        target.applyEdits([
          {
            range: { startLineNumber: line, startColumn: col, endLineNumber: line, endColumn: col },
            text: chunk,
          },
        ]);
        if (onFirstContent) {
          onFirstContent();
          onFirstContent = undefined;
        }
      },
      { signal: ctrl.signal },
    );
    if (!done || model !== target) return;
    finishFill(target, doc);
  } finally {
    if (fillCtrl === ctrl) {
      fillCtrl = null;
      filling = false;
      applyingExternal = false;
    }
  }
}

onMounted(async () => {
  const resolved = await loadMonaco();
  // Unmounted while the import was in flight — dispose nothing, mount nothing.
  if (!rootRef.value) return;
  if (docModel.value.length > LARGE_DOC_CHARS) {
    await new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())));
    if (!rootRef.value) return;
  }
  mod = resolved;

  // Empty first, filled after `create`: the doc never enters the editor's construction path.
  model = mod.editor.createModel('', languageId(mod));
  editor = mod.editor.create(rootRef.value, {
    ...applyBaseOptions(),
    model,
    theme: KIRA_EDITOR_THEME,
  });
  decorations = editor.createDecorationsCollection();
  if (props.singleLine) {
    actionDisposables.push(
      editor.addAction({
        id: SUBMIT_ACTION,
        label: SUBMIT_ACTION,
        keybindings: [mod.KeyCode.Enter],
        keybindingContext: '!suggestWidgetVisible',
        run: () => emit('submit'),
      }),
    );
  }
  if (props.saveCommand) {
    actionDisposables.push(
      editor.addAction({
        id: SAVE_ACTION,
        label: SAVE_ACTION,
        keybindings: [mod.KeyMod.CtrlCmd | mod.KeyCode.KeyS],
        run: () => emit('save'),
      }),
    );
  }
  wrapDisposable = attachWrapOnType(editor, model);
  registerProviders(mod, languageId(mod));
  model.onDidChangeContent(() => {
    if (filling) return;
    if (!applyingExternal) docModel.value = model?.getValue() ?? '';
    repaintRanges();
    if (mod) scheduleLint(mod);
    updateDebugHook();
  });
  // The pending <pre> is removed once Monaco's own DOM holds the first content (Vue batches the
  // v-if), so there is never a window with neither showing real content.
  void fillModel(docModel.value, () => {
    previewShown.value = false;
  }).then(() => {
    if (!editor) return;
    editor.layout();
    if (props.cursor && model) {
      editor.setPosition(model.getPositionAt(Math.min(props.cursor, model.getValueLength())));
    }
    if (props.scrollTop) editor.setScrollTop(props.scrollTop);
    emit('update:contentHeight', editor.getContentHeight());
    requestAnimationFrame(() => requestAnimationFrame(() => emit('ready')));
  });
  editor.onDidScrollChange((e) => {
    if (e.scrollTopChanged) emit('update:scrollTop', e.scrollTop);
  });
  editor.onDidContentSizeChange((e) => {
    if (e.contentHeightChanged) emit('update:contentHeight', e.contentHeight);
  });
  editor.onDidChangeCursorPosition((e) => {
    if (!model) return;
    emit('update:cursor', model.getOffsetAt(e.position));
  });
});

onUnmounted(() => {
  cancelFill();
  runLint.cancel();
  decorations?.clear();
  decorations = null;
  completionDisposable?.dispose();
  completionDisposable = null;
  hoverDisposable?.dispose();
  hoverDisposable = null;
  wrapDisposable?.dispose();
  wrapDisposable = null;
  for (const d of actionDisposables) d.dispose();
  actionDisposables = [];
  editor?.dispose();
  editor = null;
  model?.dispose();
  model = null;
});

// tests/ui's own text-reading seam (P60a §9.1/OQ-3) — Monaco virtualises `.view-lines`, so a full
// read needs the model, not the DOM. Compiled out of every non-debug build, matching
// `vite.config.ts`'s own `__KIRA_DEBUG_HOOKS__` convention (main.ts's `window.__kira*` hooks).
function updateDebugHook(): void {
  if (!__KIRA_DEBUG_HOOKS__) return;
  rootRef.value?.setAttribute('data-kira-editor-text', model?.getValue() ?? '');
}

defineExpose({
  focus: (): void => editor?.focus(),
  // §4.9: `getTargetAtClientPoint` + `getOffsetAt` — `null` when the point falls outside any
  // character, matching `EditorView.posAtCoords`'s own contract.
  posAtCoords: (x: number, y: number): number | null => {
    if (!editor || !model) return null;
    const target = editor.getTargetAtClientPoint(x, y);
    return target?.position ? model.getOffsetAt(target.position) : null;
  },
  scrollRangeIntoView: (from: number, to: number): void => {
    if (!editor || !model) return;
    const start = model.getPositionAt(from);
    const end = model.getPositionAt(to);
    editor.revealRangeInCenter({
      startLineNumber: start.lineNumber,
      startColumn: start.column,
      endLineNumber: end.lineNumber,
      endColumn: end.column,
    });
  },
  setCursor: (pos: number): void => {
    if (!editor || !model) return;
    const clamped = Math.min(Math.max(0, pos), model.getValue().length);
    const position = model.getPositionAt(clamped);
    editor.setPosition(position);
    editor.revealPositionInCenter(position);
  },
  setDoc: applyExternalDoc,
  // Inserts at the caret (end of text if none), spaced off a preceding word, and refocuses.
  insertText: (text: string): void => {
    if (!editor || !model) return;
    const caret = editor.getPosition();
    const offset = caret ? model.getOffsetAt(caret) : model.getValueLength();
    const before = model.getValue().slice(0, offset);
    const insert = (before !== '' && !/[\s(]$/.test(before) ? ' ' : '') + text;
    const at = model.getPositionAt(offset);
    editor.executeEdits('insert-text', [
      {
        range: {
          startLineNumber: at.lineNumber,
          startColumn: at.column,
          endLineNumber: at.lineNumber,
          endColumn: at.column,
        },
        text: insert,
      },
    ]);
    editor.setPosition(model.getPositionAt(offset + insert.length));
    editor.focus();
  },
});

// The prop alone cannot express "write this again": `doc` is a string, so a value equal to the one
// already bound triggers no watcher, and the editor keeps text the owner has since replaced
// (P89 §5). `setDoc` is that same write, reachable imperatively.
function applyExternalDoc(doc: string): void {
  if (!editor || !model) return;
  // Guards the editable round trip — same as `CodeMirrorHost.vue:329`. While the model is
  // untouched since the last write, compare against the remembered string (no `getValue()` copy).
  const unchanged =
    model.getVersionId() === lastAppliedVersionId ? lastAppliedDoc : model.getValue();
  if (doc === unchanged) return;
  const currentPosition = editor.getPosition();
  const priorOffset = currentPosition ? model.getOffsetAt(currentPosition) : 0;
  if (props.readonly) {
    // Nothing to undo on a read-only host: `setValue`/chunked `applyEdits` keep no copy of the
    // previous doc. Large docs stream in; the position reset below applies to the first chunk.
    void fillModel(doc);
  } else {
    // §4.7: the hard undo boundary around every editable external write — both
    // `pushStackElement()` calls are mandatory (one alone leaves the write mergeable on one side);
    // `setValue` would discard the whole undo stack.
    cancelFill();
    applyingExternal = true;
    model.pushStackElement();
    model.pushEditOperations(null, [{ range: model.getFullModelRange(), text: doc }], () => null);
    model.pushStackElement();
    applyingExternal = false;
    lastAppliedDoc = doc;
    lastAppliedVersionId = model.getVersionId();
  }
  if (props.keepSelectionOnExternalSync) {
    const clamped = Math.min(priorOffset, doc.length);
    const position = model.getPositionAt(clamped);
    editor.setPosition(position);
    editor.revealPositionInCenter(position);
  } else {
    editor.setPosition({ lineNumber: 1, column: 1 });
    editor.setScrollTop(0);
  }
}

watch(
  () => docModel.value,
  (doc) => applyExternalDoc(doc),
);

watch(
  () => [props.language, props.sqlDialect, props.monacoLanguage?.id],
  () => {
    if (!mod || !model) return;
    const id = languageId(mod);
    mod.editor.setModelLanguage(model, id);
    registerProviders(mod, id);
  },
);

watch(
  () => props.readonly,
  (readOnly) => {
    editor?.updateOptions({ readOnly, domReadOnly: readOnly });
  },
);

watch(
  () => [props.autocomplete, props.completionSources, props.asyncCompletionSources],
  () => {
    if (!mod) return;
    editor?.updateOptions({
      quickSuggestions: Boolean(props.autocomplete),
      suggestOnTriggerCharacters: Boolean(props.autocomplete),
      wordBasedSuggestions: wordBasedSuggestions(),
    });
    registerProviders(mod, languageId(mod));
  },
);

watch(
  () => props.lintSource,
  () => {
    if (mod) scheduleLint(mod);
  },
);

watch(
  () => props.hoverSource,
  () => {
    if (mod) registerProviders(mod, languageId(mod));
  },
);

watch(
  () => props.rangeHighlights,
  () => repaintRanges(),
);

watch(
  () => props.autoCloseBrackets,
  (on) => {
    editor?.updateOptions({
      autoClosingBrackets: on ? 'languageDefined' : 'never',
      autoClosingQuotes: on ? 'languageDefined' : 'never',
    });
  },
);

watch(
  () => settingsStore.appearance.wordWrap,
  () => {
    editor?.updateOptions({ wordWrap: resolveWordWrap() });
  },
);

watch(
  () => [settingsStore.appearance.fontFamily, settingsStore.appearance.fontSize],
  () => {
    if (!editor) return;
    editor.updateOptions({
      fontFamily: settingsStore.appearance.fontFamily,
      fontSize: settingsStore.appearance.fontSize,
    });
    editor.layout();
  },
);
</script>

<template>
  <!-- A single root, not the preview <pre> and this <div> as two siblings — a multi-root template
       gets no automatic $attrs fallthrough at all (Vue silently drops every data-testid/class a
       call site passes on the <MonacoHost> tag, e.g. RawExchangePane's own `data-testid="http-
       wire-request-editor"`), which every mount site's own drop-in contract depends on.
       Monaco mounts into this same div (`rootRef`); the pending <pre> is a child of it, removed by
       `v-if` the instant `previewShown` flips false, which is also the instant `editor.create()`
       has already appended Monaco's own DOM into the same container — briefly (one microtask) both
       are present, never observably so.

       No `data-testid` of its own on the root, deliberately: a static attribute set here would win
       Vue's own fallthrough merge over whatever `data-testid` a call site passes on `<MonacoHost>`
       (`class`/`style` concatenate on fallthrough; every other attribute, the child's own explicit
       value wins) — exactly the drop-in contract this host exists to keep
       (`CodeMirrorHost.vue`'s own root carried no `data-testid` for the identical reason).
       `.monaco-host` (the class, which DOES merge) is what `tests/ui/support/editorText.ts` and
       every UI spec use instead to find "the Monaco host" generically inside a call site's own more
       specific one. -->
  <div
    ref="rootRef"
    class="monaco-host h-full min-h-0 overflow-hidden"
    :class="{ 'monaco-host--single-line': singleLine }"
  >
    <pre
      v-if="previewShown"
      class="m-0 font-data text-kira-md text-fg bg-bg overflow-auto"
      :class="singleLine ? 'p-0 whitespace-pre' : 'py-2 px-0 whitespace-pre-wrap'"
      v-text="previewDoc"
    ></pre>
  </div>
</template>

<style scoped>
@reference "@theme/base.css";

/* `.monaco-host` stays a bare marker: a real test dependency (tests/ui/support/editorText.ts and
   every UI spec use it to find "the Monaco host") and the anchor every :deep()/:global() rule below
   needs (no template to put a class on for Monaco's own child DOM). P110 I2-18: the
   `.monaco-host--single-line.monaco-host-pending` compound moved to a ternary on the `<pre>` in the
   template; `.kira-ed-var*` moved to the shared apps/kira-studio/frontend/src/editor/
   edDecorations.css (M8), imported once from workbench.css -- AutocompleteField.vue's overlay painted a
   byte-identical copy of the same three rules. */
.monaco-host--single-line,
.monaco-host--single-line :deep(.monaco-editor) {
  @apply h-auto;
}

.monaco-host :deep(.kira-ed-find-match) {
  @apply bg-search-match;
}

.monaco-host :deep(.kira-ed-find-match-current) {
  @apply bg-search-match-current;
}

/* §4.4/dogfooding: the hover widget's value/caption split — `theme.ts:200,213`'s own two
   registers, ported here. `:global()`, not `:deep()` — `overflowWidgetsDomNode`
   (`editor/monaco.ts`) reparents every hover/suggest widget from every MonacoHost instance to one
   shared `document.body`-level container, so they are never a DOM descendant of `.monaco-host` to
   scope a selector against. */
:global(.monaco-hover) {
  @apply z-(--kira-z-tooltip);
}

:global(.monaco-hover .hover-contents pre) {
  /* P110 B37: padding: 4px 6px -> py-1 px-1.5 (audit's own literal-px note). */
  @apply bg-field border border-border rounded-kira-sm py-1 px-1.5;
}

:global(.monaco-hover .hover-contents p) {
  @apply font-ui text-kira-md text-muted-foreground;
}

:global(.suggest-widget) {
  @apply z-(--kira-z-tooltip);
}
</style>
