import { t } from '@theme/i18n';
import { useEventListener } from '@vueuse/core';
import { revealElement } from '@workbench/behaviour/focus';
import { registerCommand } from '@workbench/shortcuts/commands';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { computed, ref } from 'vue';
import { useSettingsStore } from '../../state/settings';
import { monacoLanguageFor } from './language';
import { renderMarkdownReading } from './markdownReading';
import {
  getOrCreateModel,
  KIRA_EDITOR_THEME,
  type MonacoModule,
  overflowWidgetsContainer,
} from './monaco';

type StandaloneEditor = import('monaco-editor').editor.IStandaloneCodeEditor;
type EditorOptions = import('monaco-editor').editor.IEditorOptions &
  import('monaco-editor').editor.IGlobalEditorOptions;

// A tab switch destroys and remounts the view (MainView's own v-if), which would re-run
// markdown-it and rebuild the DOM every time. Cached by tab id at module level so it survives the
// remount, and by the exact source text so a file that changed on disk between mounts misses.
// Dropped with the tab through registerTabRuntimeCleanup.
const markdownHtmlByTabId = new Map<string, { content: string; html: string }>();
registerTabRuntimeCleanup((tabId) => {
  markdownHtmlByTabId.delete(tabId);
});

/** Options for the default read-only state. */
export const READ_OPTIONS: EditorOptions = { readOnly: true, domReadOnly: true };

/** Options for edit mode: writable, with every editor extra off (no completion, hints, hover,
 *  auto-closing, formatting or validation). Plain indentation, undo/redo and find stay. */
export const EDIT_OPTIONS: EditorOptions = {
  readOnly: false,
  domReadOnly: false,
  quickSuggestions: false,
  suggestOnTriggerCharacters: false,
  wordBasedSuggestions: 'off',
  snippetSuggestions: 'none',
  tabCompletion: 'off',
  acceptSuggestionOnEnter: 'off',
  inlineSuggest: { enabled: false },
  parameterHints: { enabled: false },
  hover: { enabled: 'off' },
  lightbulb: { enabled: 'off' as import('monaco-editor').editor.ShowLightbulbIconMode },
  codeLens: false,
  links: false,
  colorDecorators: false,
  occurrencesHighlight: 'off',
  formatOnType: false,
  formatOnPaste: false,
  autoClosingBrackets: 'never',
  autoClosingQuotes: 'never',
  autoClosingDelete: 'never',
  autoClosingOvertype: 'never',
  autoSurround: 'never',
  autoIndent: 'keep',
  renderValidationDecorations: 'off',
};

export interface ReadonlyViewerOptions {
  tabId: string;
  path: string;
  markdownReading: boolean;
  /** Persists the Source/Reading choice into the tab state. */
  persistReading: (reading: boolean) => void;
  /** Prefix of the toggle's test ids: `<prefix>-view-source` and `<prefix>-view-reading`. */
  testidPrefix: string;
}

/** The read-only Monaco viewer both file views share: the editor option block, the model hook, the
 *  find command and the markdown Source/Reading toggle with its render cache. The views keep their
 *  own file reads and extras (blame, reveal). */
export function useReadonlyFileViewer(o: ReadonlyViewerOptions) {
  const settingsStore = useSettingsStore();
  const container = ref<HTMLElement | null>(null);
  const readingPane = ref<HTMLElement | null>(null);
  const fileText = ref('');
  const renderedHtml = ref<string | null>(null);
  const isMarkdown = computed(() => monacoLanguageFor(o.path) === 'markdown');
  const view = ref<'source' | 'reading'>(o.markdownReading ? 'reading' : 'source');
  const viewOptions = computed(() => [
    {
      value: 'source' as const,
      label: t('common.field.source.label'),
      testid: `${o.testidPrefix}-view-source`,
    },
    {
      value: 'reading' as const,
      label: t('space.repo.view.reading.label'),
      testid: `${o.testidPrefix}-view-reading`,
    },
  ]);
  let editor: StandaloneEditor | null = null;
  let unregisterFind: (() => void) | null = null;

  // Rendered lazily on the first switch to Reading: a markdown file opened and never toggled never
  // pays markdown-it's import cost.
  async function ensureMarkdownRendered(): Promise<void> {
    if (renderedHtml.value !== null || !fileText.value) return;
    const cached = markdownHtmlByTabId.get(o.tabId);
    if (cached !== undefined && cached.content === fileText.value) {
      renderedHtml.value = cached.html;
      return;
    }
    const html = await renderMarkdownReading(fileText.value);
    markdownHtmlByTabId.set(o.tabId, { content: fileText.value, html });
    renderedHtml.value = html;
  }

  function onViewChange(next: 'source' | 'reading'): void {
    view.value = next;
    o.persistReading(next === 'reading');
    if (next === 'reading') {
      void ensureMarkdownRendered();
    } else {
      // automaticLayout re-measures off a ResizeObserver, which fires when v-show brings the
      // container back from display: none; the explicit call is a cheap safeguard on the one
      // action that toggles it.
      editor?.layout();
    }
  }

  // Every anchor click is neutralised: the webview has no back button, so an accidental navigation
  // would strand the user. A same-page #anchor still scrolls to a heading renderMarkdownReading
  // gave an id; any other target stays inert, surfaced through the link's own title.
  function onReadingClick(event: MouseEvent): void {
    const anchor = (event.target as HTMLElement).closest('a');
    if (!anchor) return;
    event.preventDefault();
    const href = anchor.getAttribute('href');
    if (!href?.startsWith('#')) return;
    const id = decodeURIComponent(href.slice(1));
    if (!id) return;
    const target = readingPane.value?.querySelector(`#${CSS.escape(id)}`);
    if (target) revealElement(target, { block: 'start' });
  }
  useEventListener(readingPane, 'click', onReadingClick);

  /** Records the file text and, for markdown opened on Reading, starts rendering it. */
  function setText(text: string): void {
    if (text !== fileText.value) renderedHtml.value = null;
    fileText.value = text;
    if (isMarkdown.value && view.value === 'reading') void ensureMarkdownRendered();
  }

  /** Mounts a read-only editor in `container` over the model at `uri`. Every instance is
   *  readOnly/domReadOnly: neither the keyboard nor a paste can mutate a model. No minimap,
   *  suggestions, codeLens or validation decorations; this is a viewer, not an editing surface. */
  function createEditor(mod: MonacoModule, uri: string, text: string): StandaloneEditor | null {
    if (!container.value) return null;
    const model = getOrCreateModel(mod, uri, text, monacoLanguageFor(o.path));
    const created = mod.editor.create(container.value, {
      model,
      theme: KIRA_EDITOR_THEME,
      ...READ_OPTIONS,
      automaticLayout: true,
      minimap: { enabled: false },
      quickSuggestions: false,
      wordBasedSuggestions: 'off',
      parameterHints: { enabled: false },
      codeLens: false,
      renderValidationDecorations: 'off',
      scrollBeyondLastLine: false,
      fontFamily: settingsStore.appearance.fontFamily,
      fontSize: settingsStore.appearance.fontSize,
      // Keeps the hover and find widgets inside the shared overflow container under document.body
      // rather than this tab's possibly clipped panel.
      fixedOverflowWidgets: true,
      overflowWidgetsDomNode: overflowWidgetsContainer(),
    });
    editor = created;
    // Monaco ships its own find widget; the view's whole in-file search is binding the app-wide
    // view.find command to it.
    unregisterFind = registerCommand('view.find', () => {
      created.focus();
      void created.getAction('actions.find')?.run();
    });
    return created;
  }

  /** Releases the find binding and the editor reference; the view drops the widget itself. */
  function dispose(): void {
    unregisterFind?.();
    unregisterFind = null;
    editor = null;
  }

  return {
    container,
    readingPane,
    isMarkdown,
    view,
    viewOptions,
    renderedHtml,
    setText,
    createEditor,
    onViewChange,
    dispose,
  };
}
