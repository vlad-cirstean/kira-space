import { t } from '@theme/i18n';
/**
 * The repo file viewer's own git-blame annotation: injected text at the end of
 * the cursor's line, `<author>, <age> · <subject>`. The request lifecycle (debounce, cache,
 * cancellation, `repo.changed` invalidation) lives in `blameLine.ts`'s `createBlameLineController`,
 * shared with the status-bar widget (`RepoFileView.vue`) — this module is only the Monaco
 * decoration, hover and reveal-in-graph action rendering that controller's `state`.
 */
import { watch } from 'vue';
import {
  type BlameLineController,
  type BlameLineState,
  blameLineText,
  blameLineTooltip,
} from './blameLine';
import type { MonacoModule } from './monaco';

type CodeEditor = import('monaco-editor').editor.IStandaloneCodeEditor;
type DeltaDecoration = import('monaco-editor').editor.IModelDeltaDecoration;

export interface BlameAnnotationHandle {
  dispose(): void;
}

type LineState = Extract<BlameLineState, { kind: 'uncommitted' | 'resolved' }>;

function annotationText(state: LineState): string {
  return state.kind === 'uncommitted'
    ? t('common.changes.uncommitted.label')
    : blameLineText(state);
}

function hoverMessage(state: LineState): { value: string }[] {
  if (state.kind === 'uncommitted') return [{ value: t('space.repo.blame.uncommitted.tooltip') }];
  const { subject, author, date } = blameLineTooltip(state);
  return [subject, t('common.commaPair.label', { first: author, second: date })].map((value) => ({
    value,
  }));
}

/** Attaches the blame layer to an already-mounted, read-only file editor. Call once per mount
 *  (`RepoFileView.vue`); `dispose()` on unmount or when the `inlineBlame` setting turns off. Does
 *  not own `controller` — the caller disposes that separately, since the status-bar widget shares
 *  it and may outlive this renderer (`inlineBlame` off while the tab stays mounted). */
export function attachBlameAnnotation(
  mod: MonacoModule,
  editor: CodeEditor,
  controller: BlameLineController,
): BlameAnnotationHandle {
  const collection = editor.createDecorationsCollection([]);

  function paint(state: BlameLineState): void {
    if (state.kind === 'none') {
      collection.set([]);
      return;
    }
    const model = editor.getModel();
    if (!model || state.line > model.getLineCount()) {
      collection.set([]);
      return;
    }
    const maxCol = model.getLineMaxColumn(state.line);
    const decoration: DeltaDecoration = {
      range: new mod.Range(state.line, maxCol, state.line, maxCol),
      options: {
        after: {
          // A few leading spaces so the annotation never butts against the code.
          content: `    ${annotationText(state)}`,
          inlineClassName: 'kira-blame-inline',
          // §3 point 2: never affects letter spacing — the annotation sits past the line's last
          // column, so it must never shift the glyph grid.
        },
        hoverMessage: hoverMessage(state),
      },
    };
    collection.set([decoration]);
  }

  const stopWatch = watch(controller.state, paint, { immediate: true });

  // §4.5: context-menu/keybinding route to the click-through — C11 §13's own precedent for why
  // this is an editor action and not a trusted-hover command link or a raw DOM click listener on
  // the injected text (§4.5's full reasoning).
  const revealAction = editor.addAction({
    id: 'kira.blame.revealCommit',
    label: t('space.repo.blame.openInGraph.label'),
    contextMenuGroupId: 'kiraBlame',
    run: () => {
      const state = controller.state.value;
      if (state.kind !== 'resolved') return;
      // P75 §2.3: one request — hostHandlers.ts owns the stash/activate sequence this used to
      // duplicate (the review row's own "Open in graph" needed the identical logic).
      void controller.transport.request('graph.revealCommit', {
        repoId: controller.gitRepoId,
        sha: state.sha,
      });
    },
  });

  return {
    dispose(): void {
      stopWatch();
      revealAction.dispose();
      collection.clear();
    },
  };
}
