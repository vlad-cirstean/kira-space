import {
  applyPipeline,
  classifyReference,
  FAKE_NAMES,
  isDynamicName,
  isFakeName,
  isTransformName,
  type ReferenceKind,
  splitTemplateSpans,
  TRANSFORM_NAMES,
  type TransformName,
} from '@kira/api-core';
import type { ApiVariable } from '@shared/domain/variables';
import { t } from '@theme/i18n';
import { compareNatural } from '@workbench/behaviour/match';
import { type Completion, templateToken } from '@workbench/editor/fieldCompletion';
import type { RangeHighlight } from '@workbench/editor/ranges';
import type { EditorCompletionSource } from '../../editor/completion';
import { type ConsoleHoverInfo, formatHoverValue } from '../../editor/hoverInfo';
import { mergeVariableRows } from './variables';

// The Api side supplies the data, in one module, from the call already being made — the finding
// that the merged values/secrets were already synchronous, already cached, and already fired on
// mount/collection/environment change by HttpRequestView.vue (now each request view's own
// useVariableRows observers, passed straight into variableSupport below). This module builds three
// pure, DOM-free lookups over that same data for the editor seams to call: colouring
// (rangeHighlights), the hover panel's lines (hoverAt), and the `{{…}}` completion list
// (candidates). May import @kira/api-core and api/state/variables.ts; may not import
// `workbench/**` (biome.json).
//
// biome.json's per-directory noRestrictedImports bans views/grpcrequest/** from reaching into
// views/httprequest/**, and gRPC needs this module's exports too. api/state/ is the precedent
// already set for a module both protocol views need: this file imports only @kira/api-core,
// api/state/variables.ts (a sibling, now) and two type-only imports, none of which are
// views/**/project/**/workbench/**, so the move is legal in the direction that matters.

const HOVER_VALUE_MAX_LENGTH = 200;

// D2's three classes (P60a: MonacoHost.vue's own scoped `.kira-ed-var*`, renamed from
// `editor/theme.ts`'s `.cm-kira-var*` — same values, byte-identical visual result): resolved and a
// catalogued dynamic reference (both "this will produce a value") share `.kira-ed-var`; an
// uncatalogued dynamic reference is painted exactly like `unknown` (D2's own table: "unknown, or
// an uncatalogued `{{$dynamic}}`"). P17 D12: "catalogued" now means either spelling —
// `isDynamicName` ($-prefixed) or `isFakeName` (fake.-prefixed).
function classFor(name: string, kind: ReferenceKind): string {
  switch (kind) {
    case 'resolved':
      return 'kira-ed-var';
    case 'deferred':
      return 'kira-ed-var-secret';
    case 'dynamic':
      return isDynamicName(name) || isFakeName(name) ? 'kira-ed-var' : 'kira-ed-var-unknown';
    case 'unknown':
      return 'kira-ed-var-unknown';
  }
}

// A name that contains `|` but never parsed as a pipeline (`classifyReference` sees the whole
// `token | base46` string as the name) means at least one segment after the first is not a
// recognised transform. Names the first offending one, or the first segment itself when it was the
// empty-name case (`{{ | upper}}`).
function badTransformMessage(fullName: string): string {
  const segments = fullName.split('|').map((s) => s.trim());
  const rest = segments.slice(1);
  const bad = rest.find((s) => !isTransformName(s)) ?? (segments[0] === '' ? '' : rest.at(-1));
  return t('studio.api.hover.badTransform.hint', {
    name: bad ?? '',
    transforms: TRANSFORM_NAMES.join(', '),
  });
}

/** Which scope a resolved name came from, for the hover's second line — tests membership in
 *  the caller's own environment rows directly rather than widening `mergeVariableRows` with a
 *  parallel `origin` map: the one call site that needs this is here, and widening a function
 *  `send()` also depends on for a tooltip caption is the wrong direction of dependency. */
function scopeOf(name: string, environmentRows: ApiVariable[]): 'collection' | 'environment' {
  return environmentRows.some((v) => v.name === name) ? 'environment' : 'collection';
}

export interface VariableSupport {
  rangeHighlights: (doc: string) => readonly RangeHighlight[];
  /** `null` when `offset` is not inside any `{{...}}` reference — closes/never opens the hover.
   *  AutocompleteField.vue's own separate hover panel (the plain `<input>` fields — URL bar,
   *  headers, query params) is this function's one remaining direct caller: it renders every line
   *  in one flat, undifferentiated list, unaffected by hoverInfo's value/caption split below,
   *  which is scoped to the CodeMirror-native tooltip (editor/hover.ts) the request/message body
   *  editors use instead. */
  hoverAt: (text: string, offset: number) => string[] | null;
  /** The same lookup as `hoverAt`, but keeping a resolved reference's actual value (`value`,
   *  pretty-printed with editor/hover.ts's own `formatHoverValue` when it parses as JSON) separate
   *  from its explanation (`lines` — the caption `variableHoverSource` renders underneath it).
   *  `value` is `undefined` for every other reference kind (deferred/dynamic/unknown never had a
   *  value to show, only ever an explanation) — `hoverAt` above is a thin wrapper over this
   *  (`value` prepended into `lines` when present) rather than a second copy of the same per-kind
   *  switch, so the two can never drift apart on what a reference means. */
  hoverInfo: (text: string, offset: number) => HoverInfo | null;
  /** P17 D13(b): a function of the token's own context, not a static array — the list depends on
   *  whether the caret is before or after a `|` inside the reference. */
  candidates: (ctx: { text: string; from: number; word: string }) => Completion[];
}

interface HoverInfo {
  /** The reference's own value, distinct from its explanation — present only for a 'resolved'
   *  reference with an actual value to show (never for deferred/dynamic/unknown, which have only
   *  ever had an explanation). */
  value?: string;
  /** The explanation — the *entire* tooltip content when `value` is absent, a caption underneath
   *  it otherwise. */
  lines: string[];
}

// P17 D13(b): "after a `|`" is decided the same way templateToken decides "inside an unclosed
// reference" — from the nearest unclosed `{{` at or before `from`, is there already a `|`
// between it and the caret.
function isAfterPipe(text: string, from: number): boolean {
  const before = text.slice(0, from);
  const open = before.lastIndexOf('{{');
  if (open === -1) return false;
  return before.indexOf('|', open) !== -1;
}

export function variableSupport(
  collectionRows: ApiVariable[],
  environmentRows: ApiVariable[],
): VariableSupport {
  const { values, secretNames } = mergeVariableRows(collectionRows, environmentRows);

  function rangeHighlights(doc: string): readonly RangeHighlight[] {
    return splitTemplateSpans(doc)
      .filter((span) => span.isReference)
      .map((span) => ({
        from: span.from,
        to: span.to,
        class: classFor(span.name, classifyReference(span.name, values, secretNames)),
      }));
  }

  function hoverInfo(text: string, offset: number): HoverInfo | null {
    const span = splitTemplateSpans(text).find(
      (s) => s.isReference && offset >= s.from && offset < s.to,
    );
    if (!span) return null;
    const kind = classifyReference(span.name, values, secretNames);
    const pipeline = span.pipeline ?? [];
    // A reference with a pipeline gets one more line naming the chain in order — never
    // shown for `unknown` (the fallback has no pipeline by construction: the whole
    // "token | base46" string became the name instead) or for `deferred` (folded into that
    // branch's own sentence below, rather than a bare arrow chain next to "secret").
    const chainLine = pipeline.length > 0 ? [`→ ${pipeline.join(' → ')}`] : [];
    switch (kind) {
      case 'resolved': {
        const raw = values[span.name] ?? '';
        // D13(c): the renderer has the plaintext for a non-secret, so showing the piped RESULT is
        // both possible and more useful than showing the input untransformed.
        const shown = pipeline.length > 0 ? (applyPipeline(pipeline, raw) ?? raw) : raw;
        // Real-interaction fix: pretty-printed first (formatHoverValue re-indents a JSON-shaped
        // value), *then* length-capped — a value that only exceeds HOVER_VALUE_MAX_LENGTH because
        // indentation added whitespace would be a strange thing to truncate over.
        const formatted = formatHoverValue(shown);
        const truncated =
          formatted.length > HOVER_VALUE_MAX_LENGTH
            ? `${formatted.slice(0, HOVER_VALUE_MAX_LENGTH)}…`
            : formatted;
        return {
          value: truncated,
          lines: [
            scopeOf(span.name, environmentRows) === 'environment'
              ? t('studio.api.hover.environmentVariable.label')
              : t('studio.api.hover.collectionVariable.label'),
            ...chainLine,
          ],
        };
      }
      case 'deferred':
        // F5/§0.3: a secret's plaintext never enters the renderer to begin with — there is no
        // value this line could show even if it wanted to, transformed or not (P15b §4's security
        // assertion, extended to a pipeline).
        return {
          lines:
            pipeline.length > 0
              ? [t('studio.api.hover.secretPiped.hint', { transforms: pipeline.join(', ') })]
              : [t('studio.api.hover.secret.hint')],
        };
      case 'dynamic': {
        // The table also wants "the catalogue's own description" as a second line here — the
        // catalogue (api-core's dynamic/catalog.ts) deliberately carries no description strings
        // (DynamicValuesDialog.vue's own "the sample is the description", which needs an
        // async generator load this synchronous hover can't perform) — so only line 1 is shown,
        // a documented deviation rather than a second, drifting copy of that dialog's samples.
        // catalogued means either spelling.
        const catalogued = isDynamicName(span.name) || isFakeName(span.name);
        return {
          lines: catalogued
            ? [t('studio.api.hover.dynamic.hint'), ...chainLine]
            : [t('studio.api.hover.unknownDynamic.hint')],
        };
      }
      case 'unknown':
        // D3 rule 4: a name shaped like a failed pipeline (contains `|` but never parsed as one)
        // gets a second line naming exactly which segment isn't a real transform.
        return {
          lines: span.name.includes('|')
            ? [t('studio.api.hover.undefined.hint'), badTransformMessage(span.name)]
            : [t('studio.api.hover.undefined.hint')],
        };
    }
  }

  // AutocompleteField.vue's own separate hover panel — flattens `value` back onto the front of
  // `lines` so its output is byte-identical to what this function returned before hoverInfo above
  // existed (it renders every line the same, undifferentiated way regardless).
  function hoverAt(text: string, offset: number): string[] | null {
    const info = hoverInfo(text, offset);
    if (!info) return null;
    return info.value !== undefined ? [info.value, ...info.lines] : info.lines;
  }

  const varCandidates: Completion[] = [
    ...Object.keys(values).map((name) => ({
      label: name,
      detail: t('studio.api.variableCompletion.variable.label'),
      icon: 'symbol-variable',
    })),
    ...secretNames.map((name) => ({
      label: name,
      detail: t('common.secret.label'),
      icon: 'symbol-variable',
    })),
  ].sort((a, b) => compareNatural(a.label, b.label));
  // `fake.` is now the only dynamic vocabulary this app *offers*. The 58 Postman `$name` spellings
  // used to be listed right after it, tagged 'postman alias'. An already-stored
  // `{{$randomEmail}}` keeps resolving, keeps painting as a catalogued reference (classFor, above)
  // and keeps its hover — it stops being suggested, and a Postman import now rewrites it to
  // its `fake.` equivalent up front (api/state/collections.ts).
  const fakeCandidates: Completion[] = FAKE_NAMES.map((name) => ({
    label: name,
    detail: t('studio.api.variableCompletion.dynamic.label'),
    icon: 'symbol-variable',
  }));
  // D7's table, as a completion detail — a one-line reminder of what each transform does, shown
  // to the right of its name exactly where `variable`/`secret`/`dynamic` show for the other list.
  const TRANSFORM_DETAILS: Record<TransformName, string> = {
    base64: t('studio.api.variableCompletion.base64.label'),
    base64decode: t('studio.api.variableCompletion.base64decode.label'),
    upper: t('studio.api.variableCompletion.upper.label'),
    lower: t('studio.api.variableCompletion.lower.label'),
    urlencode: t('studio.api.variableCompletion.urlencode.label'),
    urldecode: t('studio.api.variableCompletion.urldecode.label'),
  };
  const transformCandidates: Completion[] = TRANSFORM_NAMES.map((name) => ({
    label: name,
    detail: TRANSFORM_DETAILS[name],
    icon: 'symbol-method',
  }));
  const nameCandidates = [...varCandidates, ...fakeCandidates];

  function candidates(ctx: { text: string; from: number; word: string }): Completion[] {
    return isAfterPipe(ctx.text, ctx.from) ? transformCandidates : nameCandidates;
  }

  return {
    rangeHighlights,
    hoverAt,
    hoverInfo,
    candidates,
  };
}

// The request body editors (RequestBodyPane.vue's raw/code hosts, GrpcRequestView.vue's
// message editor) are the single largest place a user writes {{variables}}, and until now they
// only got rangeHighlights (colouring) — hovering showed nothing, typing `{{` offered nothing.
// These two adapters give MonacoHost's hoverSource/completionSources seams the same
// VariableSupport data the plain AutocompleteField fields already use, so the behaviour matches
// exactly rather than being reimplemented.

// A pure `(doc, offset) => ConsoleHoverInfo | null` — simpler than the CodeMirror shape
// this replaced (`editor/hover.ts`'s own `buildHoverSource` existed only to turn a lookup like
// this one into a CodeMirror `HoverTooltipSource`; that glue is gone, MonacoHost.vue plugs this
// straight into its own hover provider).
//
// Real-interaction fix (reported bug — the tooltip's value and its explanation ran together):
// takes `hoverInfo` (not `hoverAt`) so the value/caption split survives — `value` renders as its
// own inset block, `lines` as the caption underneath it (hover.ts's own ConsoleHoverInfo.value doc
// comment).
export function variableHoverSource(
  hoverInfo: VariableSupport['hoverInfo'],
): (doc: string, offset: number) => ConsoleHoverInfo | null {
  return (doc, offset) => {
    const info = hoverInfo(doc, offset);
    // hoverInfo has no notion of the reference's own span (AutocompleteField.vue's own hover
    // panel gets away with the same simplification, its own comment: "without needing the
    // token's own span") — a point tooltip at `offset` re-triggers as the mouse moves, which is
    // harmless since the same info comes back for any offset still inside the reference.
    return info ? { from: offset, to: offset, lines: info.lines, value: info.value } : null;
  };
}

// Monaco's own CompletionItemKind covers exactly the two icons this app's own Completion.icon
// values use for a variable/transform candidate (MonacoHost.vue's own kindFor()).
function completionType(icon: string | undefined): 'variable' | 'method' | undefined {
  switch (icon) {
    case 'symbol-variable':
      return 'variable';
    case 'symbol-method':
      return 'method';
    default:
      return undefined;
  }
}

// Fires only inside an unclosed `{{…}}` (templateToken's own null elsewhere is what
// keeps this from popping up over ordinary body text) — the same tokenizer rule
// AutocompleteField's own `{{variable}}` fields already use. Checked against
// @codemirror/lang-json's own source: jsonLanguage's languageData carries no `autocomplete`
// entry at all, so there is no JSON completion source to re-add here — the array is this source
// alone for every `code` language, not just JSON, and for `raw`.
export function variableCompletionSource(
  candidates: VariableSupport['candidates'],
): EditorCompletionSource {
  return ({ doc, offset }) => {
    const token = templateToken(doc, offset);
    if (!token) return null;
    const list = candidates({ text: doc, from: token.from, word: token.word });
    if (list.length === 0) return null;
    return {
      from: token.from,
      options: list.map((c) => ({
        label: c.label,
        insert: c.insert ?? c.label,
        detail: c.detail,
        type: completionType(c.icon),
      })),
    };
  };
}
