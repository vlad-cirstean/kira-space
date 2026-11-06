import { mergeRawHeaders, parseRawRequest, type RawWarning } from '@kira/api-core';
import type { HttpBodyMode } from '@shared/domain/http';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { findHttpRequestTab, patchHttpRequestTabState } from '../tabs';

// The raw editor's own state — mirrors http/state/curl.ts's importCurlDialogState shape
// (one `open` flag plus what the dialog needs to seed itself and to Apply). The pasted/generated
// text itself is deliberately not stored here, the same reasoning ImportCurlDialog.vue's own
// comment states: nothing outside EditRawRequestDialog.vue needs to read it, so it stays a local
// ref there, seeded from `initialText` on open.
interface EditRawDialogState {
  open: boolean;
  tabId: string;
  initialText: string;
  originalBodyMode: HttpBodyMode;
  originalUrl: string;
}

export interface RawPreview {
  error: string | null;
  warnings: RawWarning[];
  /** Whenever the parsed body's mode differs from the tab's own mode before this edit — most
   *  commonly a `urlencoded` body folding into `raw` (raw HTTP has no `urlencoded`
   *  representation), but any Content-Type edit that changes which of the table rows the body
   *  lands in produces the same warning. The bytes and headers that will be sent are
   *  unaffected either way — only which editor renders them changes. */
  modeChanged: { from: HttpBodyMode; to: HttpBodyMode } | null;
}

export const useEditRawStore = defineStore('editRaw', () => {
  const state = reactive<EditRawDialogState>({
    open: false,
    tabId: '',
    initialText: '',
    originalBodyMode: 'none',
    originalUrl: '',
  });

  function openEditRawDialog(
    tabId: string,
    initialText: string,
    originalBodyMode: HttpBodyMode,
    originalUrl: string,
  ): void {
    state.open = true;
    state.tabId = tabId;
    state.initialText = initialText;
    state.originalBodyMode = originalBodyMode;
    state.originalUrl = originalUrl;
  }

  function closeEditRawDialog(): void {
    state.open = false;
  }

  /** Pure and synchronous — safe to call as often as a caller likes. EditRawRequestDialog.vue
   *  debounces its own calls into this (400ms, mirroring
   *  ImportCurlDialog.vue's identical debounce over previewCurl below) rather than calling it
   *  straight from the editor's `@update:doc`, since the raw buffer here is always the full
   *  generated request, not something typically hand-typed short. */
  function previewRaw(text: string): RawPreview {
    const result = parseRawRequest(text, state.originalUrl);
    if ('error' in result) {
      return { error: result.error, warnings: [], modeChanged: null };
    }
    const modeChanged =
      result.state.bodyMode !== state.originalBodyMode
        ? { from: state.originalBodyMode, to: result.state.bodyMode }
        : null;
    return { error: null, warnings: result.warnings, modeChanged };
  }

  /** Apply patches the *current* tab — never a fresh one (submitImportCurl's own
   *  openApiRequestTab is deliberately not called here), because this is the current request being
   *  re-authored, not a new one. Substitution still applies at send, unchanged: after this, the tab
   *  is an ordinary tab, and send() runs its usual two-stage resolution over whatever `{{name}}`
   *  references the hand-edited text carried (the whole point of parsing back into the model rather
   *  than sending the buffer verbatim). Returns false (leaving the dialog open) only for a parse
   *  error the Apply button should already have disabled against. */
  function applyEditRaw(text: string): boolean {
    const result = parseRawRequest(text, state.originalUrl);
    if ('error' in result) return false;
    // The editor shows only enabled, named rows; merge keeps the disabled/blank rows and descriptions.
    const current = findHttpRequestTab(state.tabId)?.state.headers ?? [];
    patchHttpRequestTabState(state.tabId, {
      ...result.state,
      headers: mergeRawHeaders(current, result.state.headers),
    });
    closeEditRawDialog();
    return true;
  }

  return { ...toRefs(state), openEditRawDialog, closeEditRawDialog, previewRaw, applyEditRaw };
});
