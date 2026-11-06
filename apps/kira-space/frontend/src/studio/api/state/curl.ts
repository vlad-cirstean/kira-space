import {
  applySecretValues,
  type CurlWarning,
  type ParsedCurl,
  parseCurl,
  type ResolvedRequest,
  toCurl,
} from '@kira/api-core';
import type { HttpCodeLanguage } from '@shared/domain/http';
import { t } from '@theme/i18n';
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { openApiRequestTab, patchHttpRequestTabState } from '../tabs';
import { loadVariableRows } from './apiQueries';
import { createRevealExpiry } from './revealExpiry';
import { useVariableSetStore } from './variables';

const CODE_LABEL: Readonly<Record<HttpCodeLanguage, string>> = {
  javascript: 'JavaScript',
  json: 'JSON',
  html: 'HTML',
  xml: 'XML',
};

function bodySummary(state: ParsedCurl['state']): string | null {
  switch (state.bodyMode) {
    case 'none':
      return null;
    case 'raw':
      return t('studio.api.curl.summary.rawBody.label');
    case 'code':
      return t('studio.api.curl.summary.codeBody.label', {
        language: CODE_LABEL[state.codeLanguage],
      });
    case 'urlencoded':
      return t('studio.api.curl.summary.formBody.label');
    case 'formdata':
      return t('studio.api.curl.summary.multipartBody.label');
    case 'file':
      return t('studio.api.curl.summary.binaryBody.label');
  }
}

/** A URL that still carries an unresolved `{{var}}` fails `new URL()` — this is a live preview
 *  over untrusted pasted text, so the summary falls back to the raw string rather than throwing. */
function urlSummary(url: string): string {
  if (url === '') return t('studio.api.curl.summary.noUrl.label');
  try {
    const u = new URL(url);
    return `${u.host}${u.pathname}`;
  } catch {
    return url;
  }
}

/** D12: the live one-line summary — "POST · api.example.com/v1/orders · 3 headers · JSON body". */
function summarizeParsed(state: ParsedCurl['state']): string {
  const parts = [state.method, urlSummary(state.url)];
  if (state.headers.length > 0) {
    parts.push(t('studio.api.curl.summary.headers.label', { count: state.headers.length }));
  }
  const body = bodySummary(state);
  if (body) parts.push(body);
  return parts.join(' · ');
}

export interface CurlPreview {
  summary: string;
  warnings: CurlWarning[];
  error: string | null;
}

// The Import-from-curl dialog's own store — mirrors http/state/dynamicValues.ts's shape
// (one `open` flag, nothing tab-scoped). The pasted text itself is deliberately not stored here:
// nothing outside ImportCurlDialog.vue needs to read it, so it stays a local ref there, the same
// way SaveRequestDialog.vue keeps its own name/target refs local rather than in the save dialog
// store's own state.
export const useImportCurlStore = defineStore('importCurl', () => {
  const state = reactive({ open: false });

  function openImportCurlDialog(): void {
    state.open = true;
  }

  function closeImportCurlDialog(): void {
    state.open = false;
  }

  /** parseCurl is pure and synchronous, so there is nothing to cache — but not free enough to
   *  call on every keystroke unthrottled: ImportCurlDialog.vue debounces its own calls
   *  into this (400ms). An empty paste is neither an error nor a preview: the dialog shows
   *  nothing yet. */
  function previewCurl(text: string): CurlPreview {
    if (text.trim() === '') return { summary: '', warnings: [], error: null };
    const parsed = parseCurl(text);
    if ('error' in parsed) return { summary: '', warnings: [], error: parsed.error };
    return { summary: summarizeParsed(parsed.state), warnings: parsed.warnings, error: null };
  }

  /** D12/F14: opens a fresh 'http-request' tab and patches it — never the tab the user might already
   *  be mid-edit on. Returns false (and leaves the dialog open) only for a parse error the Import
   *  button should already have disabled against. */
  function submitImportCurl(text: string): boolean {
    const parsed = parseCurl(text);
    if ('error' in parsed) return false;
    const id = openApiRequestTab();
    patchHttpRequestTabState(id, parsed.state);
    closeImportCurlDialog();
    return true;
  }

  return {
    ...toRefs(state),
    openImportCurlDialog,
    closeImportCurlDialog,
    previewCurl,
    submitImportCurl,
  };
});

/** The paste path's own sibling to submitImportCurl above. Same parser, different target:
 *  this patches the tab the user pasted into, because that is what they aimed at — a paste into
 *  *this* request's URL bar is an explicit gesture at *this* request, unlike the dialog, which is
 *  reached from the panel with no tab in mind and so deliberately opens a fresh one.
 *
 *  Returns false on a parse error, which the caller treats as "not a curl command after all" and
 *  lets the ordinary paste proceed — so a string that survives looksLikeCurlCommand but that
 *  parseCurl cannot make sense of is never silently swallowed. No state of its own, so this stays a
 *  plain exported function rather than a store method. */
export function applyCurlToTab(tabId: string, text: string): boolean {
  const parsed = parseCurl(text);
  if ('error' in parsed) return false;
  patchHttpRequestTabState(tabId, parsed.state);
  return true;
}

// ---- Copy as curl — masked by default, one gated reveal, nothing persisted ----
//
// The caller (HttpRequestView.vue) computes the frozen resolution itself, exactly as send() does
// (views/httprequest/state.ts's own resolveForExport — a views/ file may import http/, not the
// reverse), and hands the plain result to openCopyAsCurlDialog below. This module never
// reaches into views/** to get it.
//
// A separate store from the import dialog above rather than folded together: the two dialogs share
// no state and nothing calls from one into the other — "curl" is the file's own topic, not the
// stores' own concern.

interface CopyAsCurlDialogState {
  open: boolean;
  method: string;
  resolved: ResolvedRequest | null;
  defaultContentType: string;
  collectionId: string;
  environmentId: string;
  /** Every distinct secret name stage 1 deferred, in first-encountered order (D10 step 1). */
  deferredNames: string[];
  /** D10 step 3/§0.3: a second transient map with `revealedValues`' identical discipline — dropped
   *  on close, keyed by *name* here (not variable id) since that is what a `{{name}}` span carries. */
  revealedSecretValues: Record<string, string>;
  /** A reveal failure's message — this dialog's own sink, never `variablesDialogState.error`
   *  (§1.4's "one-line generalization"). */
  error: string | null;
}

export const useCopyAsCurlStore = defineStore('copyAsCurl', () => {
  const state = reactive<CopyAsCurlDialogState>({
    open: false,
    method: 'GET',
    resolved: null,
    defaultContentType: '',
    collectionId: '',
    environmentId: '',
    deferredNames: [],
    revealedSecretValues: {},
    error: null,
  });

  // Finding 5: this dialog's own revealed-secret map used to be dropped only by
  // openCopyAsCurlDialog/closeCopyAsCurlDialog — nothing here re-masked a revealed value once the
  // grace it came from had actually expired, and this is the more serious of the two remaining
  // unbounded maps: currentCurlCommand() renders the *fully substituted* command with the real
  // secret inline, and the dialog can sit open on screen indefinitely, long past the 5-minute
  // localauth grace that authorised the reveal. Dropping an expired entry here automatically
  // re-masks the rendered command, since currentCurlCommand() is already pure over this reactive
  // map.
  const revealedSecretValuesExpiry = createRevealExpiry(state.revealedSecretValues);

  // Bumped on open and close: a reveal loop from a superseded dialog stops and writes nothing.
  let openSeq = 0;

  function dropRevealed(): void {
    for (const key of Object.keys(state.revealedSecretValues)) {
      delete state.revealedSecretValues[key];
    }
    revealedSecretValuesExpiry.clearAll();
  }

  /** Re-masks the command; the shared reveal map goes too, so a later Show runs the gate again. */
  function hideSecretValues(): void {
    openSeq++;
    dropRevealed();
    useVariableSetStore().clearRevealed();
  }

  function openCopyAsCurlDialog(
    method: string,
    resolved: ResolvedRequest,
    deferredNames: readonly string[],
    defaultContentType: string,
    collectionId: string,
    environmentId: string,
  ): void {
    openSeq++;
    state.open = true;
    state.method = method;
    state.resolved = resolved;
    state.defaultContentType = defaultContentType;
    state.collectionId = collectionId;
    state.environmentId = environmentId;
    state.deferredNames = [...deferredNames];
    state.error = null;
    dropRevealed();
  }

  function closeCopyAsCurlDialog(): void {
    openSeq++;
    state.open = false;
    state.resolved = null;
    // D10: nothing generated is ever persisted — dropped on close, the same discipline
    // `revealedValues` follows.
    dropRevealed();
    // revealSecretValues below calls the *shared* revealVariable (state/variables.ts),
    // which writes into variables.ts's own revealedValues map, not this dialog's own
    // revealedSecretValues — closing only the latter left a stale entry there for
    // VariablesDialog.vue's own "already revealed" check to silently trust later, with no re-auth
    // prompt at all.
    useVariableSetStore().clearRevealed();
  }

  /** D10 step 2/4: the command for the *current* reveal state — masked (every deferred span still
   *  literal `{{name}}`) until revealSecretValues() has filled some or all of them in. Pure over the
   *  store's own reactive fields, so a `computed()` wrapping this in the dialog re-renders exactly
   *  when one of them changes. */
  function currentCurlCommand(): string {
    const { resolved, method, defaultContentType, revealedSecretValues } = state;
    if (!resolved) return '';
    const hasRevealed = Object.keys(revealedSecretValues).length > 0;
    const effective = hasRevealed ? applySecretValues(resolved, revealedSecretValues) : resolved;
    return toCurl({
      method,
      url: effective.url,
      headers: effective.headers,
      body: effective.body,
      defaultContentType,
    });
  }

  /** The precedence, over the id a deferred *name* belongs to — environment wins
   *  over collection, mirroring mergeVariableRows' own merge order (./variables.ts). async
   *  now — loadVariableRows is cache-first (queryClient.query), so this only awaits a
   *  fetch when a broadcast invalidated the cache since the dialog opened. */
  async function findSecretVariableId(
    name: string,
    collectionId: string,
    environmentId: string,
  ): Promise<string | null> {
    const envRows = await loadVariableRows('environment', environmentId);
    const env = envRows.find((v) => v.isSecret && v.name === name);
    if (env) return env.id;
    const colRows = await loadVariableRows('collection', collectionId);
    const col = colRows.find((v) => v.isSecret && v.name === name);
    return col?.id ?? null;
  }

  /** One `revealVariable` call per deferred name — the existing function, the existing
   *  four outcomes, the existing `confirmDialog` fallback and 5-minute grace. A
   *  cancelled, unavailable-and-declined, or errored reveal leaves that name out of
   *  `revealedSecretValues`, so its `{{name}}` span stays literal — nothing throws, nothing is
   *  refused. */
  async function revealSecretValues(): Promise<void> {
    const { deferredNames, collectionId, environmentId } = state;
    const mySeq = openSeq;
    state.error = null;
    for (const name of deferredNames) {
      if (state.revealedSecretValues[name] !== undefined) continue;
      const id = await findSecretVariableId(name, collectionId, environmentId);
      if (mySeq !== openSeq) return;
      if (!id) continue;
      // Branch on this call's own return value, not on the shared revealedValues map: a stale
      // success there for the same id must not mask a cancelled/unavailable/errored outcome.
      const value = await useVariableSetStore().revealVariable(id, (message) => {
        if (mySeq === openSeq) state.error = message;
      });
      if (mySeq !== openSeq) {
        useVariableSetStore().clearRevealed();
        return;
      }
      if (value !== undefined) {
        state.revealedSecretValues[name] = value;
        revealedSecretValuesExpiry.schedule(name);
      }
    }
  }

  /** D10: copy is available in both states — the masked form is a legal, useful, non-runnable
   *  command; the revealed form requires having passed the gate above first. */
  function copyCurlCommand(): void {
    void copyOrReportError(currentCurlCommand(), (message) => {
      state.error = message;
    });
  }

  return {
    ...toRefs(state),
    openCopyAsCurlDialog,
    closeCopyAsCurlDialog,
    currentCurlCommand,
    revealSecretValues,
    hideSecretValues,
    copyCurlCommand,
  };
});
