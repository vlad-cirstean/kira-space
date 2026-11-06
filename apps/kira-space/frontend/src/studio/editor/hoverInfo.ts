// `editor/hover.ts`'s other export, `buildHoverSource`, existed only to turn a pure
// `(doc, pos) => info` lookup into a `@codemirror/view` `HoverTooltipSource`; `MonacoHost.vue`'s
// own hover provider plugs a lookup of this exact shape straight in, so that glue has no
// CodeMirror-era equivalent to keep. Both call sites (`sqlHover.ts`,
// `api/state/variableCompletion.ts`) already only ever used these two exports.
export interface ConsoleHoverInfo {
  from: number;
  to: number;
  /** Rendered as separate lines, plain text — no markdown renderer, no new primitive (D8). Every
   *  existing caller (sqlHover.ts) uses this alone: line 0 as a title, the rest as a monospace
   *  detail/description list, all in one visual register — unaffected by `value` below. */
  lines: string[];
  /** The {{variable}} hover tooltip's value and its explanation ran together, all in one
   *  undifferentiated block of monospace lines, and a JSON-shaped value rendered as a single
   *  truncated line rather than something readable. Optional — every existing caller omits it
   *  and renders exactly as before. When set, this is the one thing in the tooltip meant to be
   *  *read as data*: pre-wrapped, its own inset block distinct from `lines` (rendered as a
   *  caption underneath, in the muted/secondary treatment `lines` alone never had) —
   *  variableCompletion.ts's own `hoverAt` supplies a resolved reference's value here
   *  (pretty-printed with `formatHoverValue` below when it parses as JSON) and moves what used to
   *  be the value's own first line ("environment variable", a transform chain, …) into `lines`,
   *  which MonacoHost.vue now renders as the caption instead of an undifferentiated peer line. */
  value?: string;
}

// A resolved variable's value is arbitrary text (most often a bare string, sometimes a JSON
// fragment pasted into an environment). Re-indenting it when it parses as JSON is a real
// readability win (nested braces on their own lines instead of one long truncated run); anything
// that doesn't parse is shown verbatim, unchanged, since it is not JSON to begin with (a URL,
// a token, plain prose).
export function formatHoverValue(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

// MonacoHost.vue renders every ConsoleHoverInfo/HoverInfo `lines` entry as a
// Markdown string (`supportHtml: false` blocks raw HTML, not Markdown). `lines` is built from
// database content this app doesn't control — table/column names, a column's `COMMENT ON COLUMN`
// description, a resolved variable's value — so a column comment reading `[docs](https://…)`
// rendered as a clickable link, `_x_`/`__init__`-style names rendered as italic/bold, and a
// backslash or backtick in a name or type changed the text shown. Backslash-escapes every
// Markdown syntax token, mirroring VS Code's own `escapeMarkdownSyntaxTokens` (including the
// backslash character itself, so a literal backslash already in the text can't combine with the
// next escaped character and produce a different escape than intended).
const MARKDOWN_SYNTAX_TOKENS = /[\\`*_{}[\]()#+\-.!|<>~]/g;

export function escapeMarkdownSyntaxTokens(text: string): string {
  return text.replace(MARKDOWN_SYNTAX_TOKENS, '\\$&');
}

// `value` used to be wrapped in a fixed triple-backtick fence — a variable value
// containing its own triple backtick closed that fence early, and the rest of the value rendered
// as Markdown instead of as the pre-wrapped literal text it's meant to be. CommonMark's own rule: a
// fenced code block's fence must be longer than the longest backtick run inside it (and at least 3
// backticks to be a fence at all, not inline-code syntax).
export function fenceMarkdownValue(value: string): string {
  const runs = value.match(/`+/g) ?? [];
  const longestRun = runs.reduce((max, run) => Math.max(max, run.length), 0);
  const fence = '`'.repeat(Math.max(3, longestRun + 1));
  return `${fence}\n${value}\n${fence}`;
}
