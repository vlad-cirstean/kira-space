import { t } from '@theme/i18n';
import { useDistillUiStore } from '@workbench/memory/distill/distillStore';
import { useMemoryUiStore } from '@workbench/memory/store';
import { watch } from 'vue';
import { useModeStore } from '../../state/mode';
import { useMemoryEnabled } from '../../workbench/experimental';

type CodeEditor = import('monaco-editor').editor.IStandaloneCodeEditor;

/** Adds the `Distill selection` context-menu action to a file view: the selected text opens in the
 *  Memory module's distill view while `experimental.memory` is on, with its path and line range as origin. Returns the teardown. */
export function attachDistillSelection(editor: CodeEditor, path: string): () => void {
  // A context key, not a one-time check: the action turns on and off live for an open file.
  const enabled = useMemoryEnabled();
  const key = editor.createContextKey('kira.memoryEnabled', enabled.value);
  const stop = watch(enabled, (on) => key.set(on));
  const action = editor.addAction({
    id: 'memory.distillSelection',
    label: t('memory.distill.selection.label'),
    precondition: 'editorHasSelection && kira.memoryEnabled',
    contextMenuGroupId: 'navigation',
    contextMenuOrder: 1,
    run: () => {
      const sel = editor.getSelection();
      const text = sel ? (editor.getModel()?.getValueInRange(sel) ?? '') : '';
      if (!sel || text.trim() === '') return;
      // A selection ending at column 1 of a later line does not include that line.
      const end =
        sel.endColumn === 1 && sel.endLineNumber > sel.startLineNumber
          ? sel.endLineNumber - 1
          : sel.endLineNumber;
      useDistillUiStore().openWith({ text, origin: { path, start: sel.startLineNumber, end } });
      useMemoryUiStore().view = 'distill';
      useModeStore().setMode('memory');
    },
  });
  return () => {
    stop();
    action.dispose();
  };
}
