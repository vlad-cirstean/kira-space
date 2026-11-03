import { editorForTab } from './editors';

// RepoFileView.vue applies its own `revealLine` on mount only, so a jump into a tab that is
// *already mounted and active* (the common case for a search result: two matches in the file
// you're already reading, or a go-to-definition inside the current file) moves nothing today.
// A pending reveal per tab: applied immediately when that tab's editor is live (requestReveal),
// consumed on mount otherwise (consumeReveal).
//
// Carries a column/endColumn, which repo-file's own persisted tab state (repoFileTabStateSchema)
// deliberately does not — a column is worth restoring a cursor to within a session, not across a
// restart, and widening the stored schema would turn a scroll position into a migration-shaped
// concern. Imports only ./editors (which imports ./monaco, neither imports state/*), so this
// closes no cycle.
export interface RevealRequest {
  line: number;
  column?: number;
  endColumn?: number;
}

const pending = new Map<string, RevealRequest>();

/** Applies req immediately when tabId's own editor is already mounted and live (setSelection +
 *  revealRangeInCenterIfOutsideViewport, or revealLineInCenter with no column); otherwise stores
 *  it for that tab's next mount to consume via consumeReveal. */
export function requestReveal(tabId: string, req: RevealRequest): void {
  const editor = editorForTab(tabId);
  if (!editor) {
    pending.set(tabId, req);
    return;
  }
  applyReveal(editor, req);
}

/** RepoFileView.vue's own mount hook: takes and clears tabId's own pending reveal, if any — called
 *  in preference to the tab's persisted `state.revealLine`, falling back to it when there is none. */
export function consumeReveal(tabId: string): RevealRequest | null {
  const req = pending.get(tabId) ?? null;
  if (req) pending.delete(tabId);
  return req;
}

/** A fresh editor's initial position: the pending reveal wins over the persisted line, which only
 *  applies when the editor was not restored from a cached model. */
export function applyMountReveal(
  editor: import('monaco-editor').editor.IStandaloneCodeEditor,
  pendingReveal: RevealRequest | null,
  restored: boolean,
  revealLine: number | null,
): void {
  if (pendingReveal) {
    if (pendingReveal.column === undefined) {
      editor.revealLineInCenter(pendingReveal.line);
      editor.setPosition({ lineNumber: pendingReveal.line, column: 1 });
    } else {
      const range = {
        startLineNumber: pendingReveal.line,
        startColumn: pendingReveal.column,
        endLineNumber: pendingReveal.line,
        endColumn: pendingReveal.endColumn ?? pendingReveal.column,
      };
      editor.setSelection(range);
      editor.revealRangeInCenter(range);
    }
  } else if (!restored && revealLine !== null) {
    editor.revealLineInCenter(revealLine);
    editor.setPosition({ lineNumber: revealLine, column: 1 });
  }
}

function applyReveal(
  editor: import('monaco-editor').editor.IStandaloneCodeEditor,
  req: RevealRequest,
): void {
  if (req.column === undefined) {
    editor.revealLineInCenter(req.line);
    editor.setPosition({ lineNumber: req.line, column: 1 });
    return;
  }
  const range = {
    startLineNumber: req.line,
    startColumn: req.column,
    endLineNumber: req.line,
    endColumn: req.endColumn ?? req.column,
  };
  editor.setSelection(range);
  editor.revealRangeInCenterIfOutsideViewport(range);
}
