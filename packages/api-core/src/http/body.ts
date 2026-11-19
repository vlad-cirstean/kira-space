import type { EditorLanguageId } from '@kira/shared/domain/editor';
import {
  CONTENT_TYPE_BY_CODE_LANGUAGE,
  type HttpBodyMode,
  type HttpCodeLanguage,
  type HttpHeaderState,
  type HttpRequestTabState,
} from '@kira/shared/domain/http';
import { t } from '@theme/i18n';
import type { TextRef } from '../lib/textRef';

// A UI-level body-mode selection — HTTP_BODY_MODES itself is unchanged (still six values,
// no 'json'); this is only what the segmented control in RequestBodyPane.vue renders and reads,
// layered over the same `bodyMode`/`codeLanguage` storage. See RequestBodyPane.vue's own
// `selection` computed for the mapping.
export type HttpBodySelection = HttpBodyMode | 'json';

// The body-mode segments, in the builder's own order; the visible label and tooltip live in
// the catalog (`studio.http.bodyMode.<value>`). `json` is a presentation-only segment
// between raw and code — selecting it patches `{bodyMode: 'code', codeLanguage: 'json'}`, so it
// needs no schema/wire value of its own.
export const BODY_MODE_OPTIONS: readonly {
  value: HttpBodySelection;
  testid: string;
}[] = [
  { value: 'none', testid: 'http-body-mode-none' },
  { value: 'formdata', testid: 'http-body-mode-formdata' },
  { value: 'urlencoded', testid: 'http-body-mode-urlencoded' },
  { value: 'raw', testid: 'http-body-mode-raw' },
  { value: 'json', testid: 'http-body-mode-json' },
  { value: 'code', testid: 'http-body-mode-code' },
  { value: 'file', testid: 'http-body-mode-file' },
];

// The `code` mode's sub-selector, in the same order raw's own sub-selector used to list them
// (minus Text, which is what plain `raw` now means, and minus JSON, which is its own top-level
// segment now — P15 D6).
export const CODE_LANGUAGE_OPTIONS: readonly { value: HttpCodeLanguage; label: TextRef }[] = [
  { value: 'javascript', label: { key: 'workbench.apiImport.codeLanguage.javascript.label' } },
  { value: 'html', label: { key: 'workbench.apiImport.codeLanguage.html.label' } },
  { value: 'xml', label: { key: 'workbench.apiImport.codeLanguage.xml.label' } },
];

// Which CodeMirror grammar a code sub-language renders with — only json/xml have a real grammar
// in this app (languages.ts); javascript/html render unhighlighted, exactly as they did as raw
// sub-languages before the split (no requirement beyond colour for either).
const CODE_LANGUAGE_TO_EDITOR: Readonly<Record<HttpCodeLanguage, EditorLanguageId>> = {
  javascript: 'plain',
  json: 'json',
  html: 'plain',
  xml: 'xml',
};

export function editorLanguageForCode(codeLanguage: HttpCodeLanguage): EditorLanguageId {
  return CODE_LANGUAGE_TO_EDITOR[codeLanguage];
}

/** The user's own Content-Type header value, if any enabled row sets one — case-insensitive,
 *  last one wins (mirrors client.go's own headerValue). */
export function userContentTypeHeader(headers: readonly HttpHeaderState[]): string | undefined {
  let value: string | undefined;
  for (const h of headers) {
    if (h.enabled && h.name.trim().toLowerCase() === 'content-type') value = h.value;
  }
  return value;
}

/** D7: the Content-Type Go's own default would apply for this mode — "" when the mode sends none
 *  at all (none, binary). Mirrors internal/httpclient/client.go's own Send precedence, read-only:
 *  this never talks to Go, it only states what D7 documents Go will do. */
export function defaultContentTypeFor(mode: HttpBodyMode, codeLanguage: HttpCodeLanguage): string {
  switch (mode) {
    case 'none':
    case 'file':
      return '';
    case 'raw':
      return 'text/plain';
    case 'code':
      return CONTENT_TYPE_BY_CODE_LANGUAGE[codeLanguage];
    case 'urlencoded':
      return 'application/x-www-form-urlencoded';
    case 'formdata':
      return 'multipart/form-data; boundary=…';
  }
}

// Protocol token, passed as a param so the catalog text carries no header name.
const CONTENT_TYPE = 'Content-Type';

/** D9's caption: exactly what Content-Type the send will actually carry — a hand-set header wins
 *  (D7's precedence), binary gets the explicit "no Content-Type" sentence (F3: Postman sets none
 *  either), and 'none' has nothing to caption. */
export function contentTypeCaption(
  mode: HttpBodyMode,
  codeLanguage: HttpCodeLanguage,
  userContentType: string | undefined,
): string {
  const trimmed = (userContentType ?? '').trim();
  if (trimmed !== '')
    return t('studio.http.body.contentType.fromHeader.status', {
      header: CONTENT_TYPE,
      type: trimmed,
    });
  if (mode === 'none') return '';
  if (mode === 'file')
    return t('studio.http.body.contentType.binary.status', { header: CONTENT_TYPE });
  return t('studio.http.body.contentType.auto.status', {
    header: CONTENT_TYPE,
    type: defaultContentTypeFor(mode, codeLanguage),
  });
}

/** P15 D8: what counts as "this request has a body", per mode — the tab-strip badge's predicate
 *  (state/tabKinds.ts's `badge()`). */
export function hasRequestBody(state: HttpRequestTabState): boolean {
  switch (state.bodyMode) {
    case 'none':
      return false;
    case 'raw':
      return state.body.trim() !== '';
    case 'code':
      return state.code.trim() !== '';
    case 'urlencoded':
      return state.urlEncoded.some((f) => f.enabled && f.name.trim() !== '');
    case 'formdata':
      return state.formData.some((f) => f.enabled && f.name.trim() !== '');
    case 'file':
      return state.binaryFile !== null;
  }
}
