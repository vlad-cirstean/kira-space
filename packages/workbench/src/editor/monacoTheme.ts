// `MonacoModule`/`cssVar`/`KIRA_EDITOR_THEME` used to live in each app's own editor/monaco.ts,
// byte-identical there in both apps save for one header comment — moved here, this file's own sole
// real consumer, when this file (and monacoEntry.ts alongside it) hoisted to packages/workbench.
// Each app's own monaco.ts now re-exports `cssVar`/
// `KIRA_EDITOR_THEME` from here unchanged, so every existing external consumer of either symbol
// (MonacoHost.vue, ResponseDiffDialog.vue, useDiffEditor.ts, RepoFileView.vue, …) needs no edit.
export type MonacoModule = typeof import('./monacoEntry');

// Read once from getComputedStyle against tokens.css — this app has one fixed (dark) visual
// design with no light/dark toggle today (tokens.css names each value after the VS
// Code theme key it fills, e.g. "--kira-bg" for editor.background), so `base: 'vs-dark'`
// is correct and there is only one theme to define.
export const KIRA_EDITOR_THEME = 'kira-editor';

// C6 dogfooding finding (§13.5's own live-verification pass, real WebKit — the engine the packaged
// app's WKWebView actually embeds, matching playwright.config.ts's own choice of `webkit` for
// UI-fidelity projects), **widened by a second P60a dogfooding finding**: WebKit's
// `getComputedStyle` canonicalises a custom property's own color value to whatever it considers
// its *shortest* serialization — a 3-digit hex shorthand for `#cccccc` (C6's own
// finding, `#ccc`), but a bare CSS colour *keyword* when one exactly matches, e.g.
// `#808080` comes back as the literal string `"gray"`. Monaco's `defineTheme`
// validates a token rule's `foreground` strictly as a hex string, throwing on either form
// ("Illegal value for token color: #ccc" / "... gray") and rejecting `loadMonaco()`'s own memoised
// promise *permanently* — every MonacoHost on the page is left showing its pending `<pre>` forever,
// no error surface at all (worse than C5's own described "missing worker" failure mode, since
// nothing here even logs past the one console error). Chromium does not canonicalise either way,
// which is why both forms went unnoticed until a real WebKit run.
//
// A canvas 2D context's own `fillStyle` setter accepts the full CSS `<color>` grammar (any
// keyword, any hex length, `rgb()`/`hsl()`/`color-mix()`/`color()`/...). Reading the *string* back
// out of `fillStyle` (the earlier approach here) only serializes as far as each engine's own
// `<color>` stringifier goes -- both Chromium and WebKit hand back a `color()` function string for
// a `color-mix()` token (`--kira-search-match`'s own `color(srgb 0.8 0.654902 0 / 0.25)`), which
// the old rgba-only regex below never matched, so the raw string reached Monaco's own
// `Color.fromHex` and silently resolved to `Color.red` (browser-verified, both engines). Reading
// the *rendered pixel* instead is engine-agnostic by construction: whatever the color computes to,
// `getImageData` hands back its actual RGBA bytes, no string grammar to keep up with.
let normalizeCanvasCtx: CanvasRenderingContext2D | null | undefined;

// The alpha trap: a translucent token (a
// `color-mix(… transparent)` token like `--kira-search-match`) is not opaque, so
// `editor.defineTheme`'s `colors` values -- which go through `Color.fromHex`
// (`StandaloneTheme.getColors()`, standaloneThemeService.js), accepting only
// `#RGB`/`#RGBA`/`#RRGGBB`/`#RRGGBBAA` -- need the alpha byte carried through, not dropped. Always
// emitting `#rrggbbaa` (never bare `#rrggbb`) is what lets `defineKiraTheme` map a translucent
// `--kira-*` token at all without silently painting a widget red; `ColorMap.getId`'s own regex
// (`tokenization.js`) accepts the trailing alpha pair for token `rules` foregrounds too, so one
// format serves both call sites below.
function normalizeColor(color: string): string {
  if (normalizeCanvasCtx === undefined) {
    const canvas = document.createElement('canvas');
    canvas.width = 1;
    canvas.height = 1;
    normalizeCanvasCtx = canvas.getContext('2d');
    // 'copy' replaces the destination pixel outright instead of alpha-compositing the new fill
    // over whatever the previous call left behind -- required for a translucent `color` to read
    // back as itself rather than blended with the prior draw.
    if (normalizeCanvasCtx) normalizeCanvasCtx.globalCompositeOperation = 'copy';
  }
  if (!normalizeCanvasCtx) return color; // no canvas 2D support — pass through rather than throw
  normalizeCanvasCtx.fillStyle = '#000000'; // known-good reset, so an invalid `color` leaves this
  normalizeCanvasCtx.fillStyle = color;
  normalizeCanvasCtx.fillRect(0, 0, 1, 1);
  const [r, g, b, a] = normalizeCanvasCtx.getImageData(0, 0, 1, 1).data;
  const byte = (n: number): string => n.toString(16).padStart(2, '0');
  return `#${byte(r)}${byte(g)}${byte(b)}${byte(a)}`;
}

export function cssVar(name: string, fallback: string): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return normalizeColor(value || fallback);
}

// `theme.ts`'s 283 CodeMirror lines split three ways. This file carries the first two —
// editor chrome colours and token colours — as one Monaco `defineTheme` call. The third
// (app-owned `.cm-kira-*` classes) becomes plain scoped CSS on `MonacoHost.vue` itself, renamed
// `.kira-ed-*`.

// §4.1 token-colour table — Lezer tag group -> Monarch scope, both keyed by the same
// `--kira-syntax-*` custom property `theme.ts`'s `kiraHighlightStyle` already reads.
function tokenRules(): { token: string; foreground?: string; fontStyle?: string }[] {
  const comment = cssVar('--kira-syntax-comment', '#7f9a6f');
  const string = cssVar('--kira-syntax-string', '#d29a7e');
  const number = cssVar('--kira-syntax-number', '#b4c79e');
  const keyword = cssVar('--kira-syntax-keyword', '#6e9fd8');
  const control = cssVar('--kira-syntax-control', '#c296c4');
  const property = cssVar('--kira-syntax-property', '#93c6e8');
  const name = cssVar('--kira-syntax-name', '#93c6e8');
  const fn = cssVar('--kira-syntax-function', '#d8d2a0');
  const tag = cssVar('--kira-syntax-tag', '#6e9fd8');
  const attribute = cssVar('--kira-syntax-attribute', '#93c6e8');
  const operator = cssVar('--kira-syntax-operator', '#d4d4d8');
  const punctuation = cssVar('--kira-syntax-punctuation', '#d4d4d8');
  const meta = cssVar('--kira-syntax-meta', '#8e8e97');
  const invalid = cssVar('--kira-syntax-invalid', '#e66e68');

  // `foreground` strips a leading '#' — Monaco's `editor.IStandaloneThemeData['rules']` wants the
  // bare hex, unlike `colors` below which wants the full `#RRGGBB` string.
  const bare = (hex: string): string => hex.replace(/^#/, '');

  return [
    { token: 'comment', foreground: bare(comment) },
    { token: 'string', foreground: bare(string) },
    { token: 'string.sql', foreground: bare(string) },
    { token: 'string.escape', foreground: bare(string) },
    { token: 'number', foreground: bare(number) },
    { token: 'keyword', foreground: bare(keyword) },
    // Checked and declined a separate `type.identifier` row (TS/JS emit it for any `[A-Z]…`
    // identifier): `ThemeTrieElement.match` (tokenization.js) walks the dotted scope by segment and
    // falls back to the nearest registered ancestor, so `type.identifier` already resolves to this
    // `type` rule with no extra entry needed.
    { token: 'type', foreground: bare(keyword) },
    { token: 'constant', foreground: bare(keyword) },
    { token: 'keyword.control', foreground: bare(control) },
    { token: 'variable.name', foreground: bare(property) },
    // Not dead: `ini.js` (`key=value`), `less.js`, `lua.js`, `julia.js`, `scala.js`, `abap.js` and
    // `clojure.js` all still emit a bare `key` token — several only reachable since D3 registered
    // their grammars at all (verified with a grep across every `languages/definitions/*/*.js`).
    { token: 'key', foreground: bare(property) },
    // Monaco's YAML grammar emits mapping keys as `type.yaml`, which the `type` rule above would
    // paint keyword blue, the same as `true`/`null`.
    { token: 'type.yaml', foreground: bare(property) },
    { token: 'operators.yaml', foreground: bare(operator) },
    { token: 'strong', fontStyle: 'bold' },
    { token: 'emphasis', fontStyle: 'italic' },
    // Monaco's own JSON tokenizer (`languages/features/json/tokenization.js`) emits
    // `string.key.json` for an object key and `string.value.json` for a string value — distinct
    // scopes so a package.json's keys and values no longer share one color. `string.value` is
    // explicit (not left to inherit `string`'s own rule) so it can never accidentally pick up
    // `string.key`'s color if the trie shape ever changes upstream.
    { token: 'string.key', foreground: bare(property) },
    { token: 'string.value', foreground: bare(string) },
    // D2: the decorator/annotation token `withDecorators` (monarch/decorators.ts) prepends for
    // TypeScript/JavaScript — also what Java's own grammar already emits for `@Override`, which
    // had no rule before this and fell back to plain `editor.foreground`.
    { token: 'annotation', foreground: bare(fn) },
    { token: 'attribute.name', foreground: bare(attribute) },
    { token: 'identifier', foreground: bare(name) },
    { token: 'variable', foreground: bare(name) },
    { token: 'entity.name.function', foreground: bare(fn) },
    { token: 'predefined', foreground: bare(fn) },
    { token: 'tag', foreground: bare(tag) },
    { token: 'operator', foreground: bare(operator) },
    { token: 'delimiter', foreground: bare(punctuation) },
    { token: 'delimiter.parenthesis', foreground: bare(punctuation) },
    { token: 'delimiter.bracket', foreground: bare(punctuation) },
    { token: 'metatag', foreground: bare(meta) },
    { token: 'invalid', foreground: bare(invalid) },
  ];
}

/** Defines `KIRA_EDITOR_THEME` — every editor surface in the app (repo view/diff, and every
 *  `MonacoHost.vue` mount) applies this one theme. Idempotent: `defineTheme` under the same name
 *  simply redefines it, safe to call once per `loadMonaco()` resolution. */
export function defineKiraTheme(mod: MonacoModule): void {
  mod.editor.defineTheme(KIRA_EDITOR_THEME, {
    base: 'vs-dark',
    inherit: true,
    rules: tokenRules(),
    colors: {
      'editor.background': cssVar('--kira-bg', '#1b1b1e'),
      'editor.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'editorWidget.background': cssVar('--kira-bg-elevated', '#212125'),
      'editorWidget.border': cssVar('--kira-border', '#323237'),
      'editor.selectionBackground': cssVar('--kira-select', '#1f3354'),
      'editor.lineHighlightBackground': cssVar('--kira-bg-input', '#27272c'),
      'editorLineNumber.foreground': cssVar('--kira-fg-muted', '#a1a1aa'),
      'editorCursor.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'editorGutter.background': cssVar('--kira-bg', '#1b1b1e'),
      focusBorder: cssVar('--kira-focus', '#5a8ae6'),
      // §4.5: the themed wavy underline replacing CodeMirror's hard-coded raster squiggle.
      'editorError.foreground': cssVar('--kira-error', '#e66e68'),
      'editorWarning.foreground': cssVar('--kira-warn', '#d5b449'),
      // §4.4: `.cm-tooltip-lint`'s chrome, reused for Monaco's own hover widget.
      'editorHoverWidget.background': cssVar('--kira-bg-elevated', '#212125'),
      'editorHoverWidget.border': cssVar('--kira-border', '#323237'),
      // §4.8: the completion popup's chrome — primitives.css's own floating-panel tokens.
      'editorSuggestWidget.background': cssVar('--kira-bg-elevated', '#212125'),
      'editorSuggestWidget.border': cssVar('--kira-border', '#323237'),
      'editorSuggestWidget.selectedBackground': cssVar('--kira-select', '#1f3354'),
      'editorSuggestWidget.highlightForeground': cssVar('--kira-syntax-function', '#d8d2a0'),

      // The ~16 keys above cover the editor surface itself; the context menu, the
      // rest of the suggest widget, the find/peek widgets and every reparented list read from a
      // ~40-key set `base: 'vs-dark'` otherwise answers with its own hardcoded literals (e.g. the
      // menu's `#3C3C3C` against this app's `--kira-bg-elevated` `#212125`) — visibly a different
      // palette. Every key below was read out of the Monaco stylesheet or default-style object that
      // consumes it (`base/browser/ui/menu/menu.js`'s `getMenuWidgetCSS`,
      // `editor/contrib/suggest/browser/media/suggest.css`, `platform/theme/common/colors/
      // listColors.js`), mapped from an existing `--kira-*` token — no new token, no new literal.
      'menu.background': cssVar('--kira-bg-elevated', '#212125'),
      'menu.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'menu.selectionBackground': cssVar('--kira-bg-input', '#27272c'),
      'menu.selectionForeground': cssVar('--kira-fg', '#d4d4d8'),
      'menu.separatorBackground': cssVar('--kira-border', '#323237'),
      'menu.border': cssVar('--kira-border', '#323237'),

      'editorSuggestWidget.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'editorSuggestWidget.selectedForeground': cssVar('--kira-fg', '#d4d4d8'),
      'editorSuggestWidget.focusHighlightForeground': cssVar('--kira-syntax-function', '#d8d2a0'),
      'editorSuggestWidget.selectedIconForeground': cssVar('--kira-fg-muted', '#a1a1aa'),
      'editorSuggestWidgetStatus.foreground': cssVar('--kira-fg-muted', '#a1a1aa'),

      'list.hoverBackground': cssVar('--kira-bg-input', '#27272c'),
      'list.hoverForeground': cssVar('--kira-fg', '#d4d4d8'),
      'list.activeSelectionBackground': cssVar('--kira-select', '#1f3354'),
      'list.activeSelectionForeground': cssVar('--kira-accent-fg', '#ffffff'),
      'list.focusBackground': cssVar('--kira-select', '#1f3354'),
      'list.focusOutline': cssVar('--kira-focus', '#5a8ae6'),
      'list.highlightForeground': cssVar('--kira-syntax-function', '#d8d2a0'),

      'dropdown.background': cssVar('--kira-bg-input', '#27272c'),
      'dropdown.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'dropdown.border': cssVar('--kira-border', '#323237'),

      'input.background': cssVar('--kira-bg-input', '#27272c'),
      'input.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'input.border': cssVar('--kira-border', '#323237'),

      'editorWidget.foreground': cssVar('--kira-fg', '#d4d4d8'),
      'widget.border': cssVar('--kira-border', '#323237'),
      // No plain-colour shadow token exists in the palette — `--kira-shadow`
      // is a full `box-shadow` shorthand, not a `<color>`, so they
      // cannot feed `Color.fromHex` the way this key needs. `--kira-border` (the existing
      // boundary colour) stands in rather than inventing a new literal.
      'widget.shadow': cssVar('--kira-border', '#323237'),

      'scrollbarSlider.background': cssVar('--kira-scrollbar', '#323237'),
      'scrollbarSlider.hoverBackground': cssVar('--kira-scrollbar-hover', '#8e8e97'),
      'scrollbarSlider.activeBackground': cssVar('--kira-scrollbar-active', '#8e8e97'),

      'peekViewEditor.background': cssVar('--kira-bg', '#1b1b1e'),
      'peekViewResult.background': cssVar('--kira-bg-elevated', '#212125'),
      'peekViewTitle.background': cssVar('--kira-bg-chrome', '#131315'),

      // `--kira-search-match` is a translucent `color-mix(… transparent)` token — same alpha branch
      // as the scrollbar above. `--kira-search-match-current` resolves to an opaque `--kira-warn`
      // and needs no conversion, but is routed through the same `cssVar` call for consistency.
      'editor.findMatchBackground': cssVar('--kira-search-match-current', '#d5b449'),
      'editor.findMatchHighlightBackground': cssVar('--kira-search-match', '#d5b44940'),

      'textLink.foreground': cssVar('--kira-info', '#69a1e8'),
      'textLink.activeForeground': cssVar('--kira-info', '#69a1e8'),
    },
  });
  // `colorize()` (AutocompleteField's filter overlay) paints with the *global* theme, which stays
  // Monaco's default light `vs` until an editor mounts with `theme:` set — a filter input with no
  // editor on screen rendered dark-on-dark text. Make Kira's theme the global one from definition.
  mod.editor.setTheme(KIRA_EDITOR_THEME);
}
