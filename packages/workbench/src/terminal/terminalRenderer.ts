// The sole contact point with `@xterm/xterm`, reached only through a dynamic `import()`
// (RepoTerminalView.vue's own mount) — never a static import at the top of a module Vite includes
// in the boot bundle. Static re-exports here would be pointless (nothing outside this module
// needs the xterm types), but the shape mirrors monacoEntry.ts's own recorded reasoning: everyone
// this codebase's Terminal instance touches lives in this one file.
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import { tooltipContentVariants } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { isPrimaryModifier, primaryModifierName } from '../shortcuts/keys';
import { registerTabRuntimeCleanup } from '../state/tabRuntime';
import { createFileLinkProvider } from './fileLinkProvider';

interface Attached {
  term: Terminal;
  fit: FitAddon;
  host: HTMLDivElement;
  off: () => void;
}

// `useSettingsStore`/`useTerminalsStore` used to be imported directly (each app's own
// `state/settings.ts`/`state/terminals.ts`, neither hoisted here — both are real per-app stores,
// `settings.ts` genuinely diverges in section count and `terminals.ts` needs the workspace-key
// parameterization). This module has no package-boundary way to reach either
// app's own store directly, so the two values/methods it touches are passed in instead —
// a real parameterization, not a plain move, forced by that boundary. Each app's own TerminalView
// component (which already holds both stores for its own use) passes them through unchanged. */
export interface TerminalRendererDeps {
  /** Live-read at call time (both call sites read it fresh, matching the pre-move behaviour of
   *  reading `settingsStore.appearance` directly). */
  appearance(): { fontFamily: string; fontSize: number };
  onTerminalOutput(tabId: string, cb: (bytes: Uint8Array) => void): () => void;
  writeTerminal(tabId: string, bytes: Uint8Array): void;
  /** Opens an http(s) link from terminal output; never throws. */
  openLink(url: string): void;
  /** 'modifier': Cmd-click (mac) or Ctrl-click, with a hover hint. 'tap': plain click/tap (phone). */
  linkActivation: 'modifier' | 'tap';
  /** Kira Space desktop only; absent means file paths are not links. */
  fileLinks?: TerminalFileLinks;
}

export interface FileLinkTarget {
  path: string;
  line: number | null;
  col: number | null;
}

export interface TerminalFileLinks {
  /** One answer per path: does it name an openable file, against tabId's live cwd. */
  resolve(tabId: string, paths: string[]): Promise<boolean[]>;
  /** Opens one target in Kira Space; never throws (announces a failure). */
  open(tabId: string, target: FileLinkTarget): void;
}

// Mirrors the Go OpenExternalURL rule; Go stays the authority.
function isWebUrl(uri: string): boolean {
  if (!URL.canParse(uri)) return false;
  const u = new URL(uri);
  return (u.protocol === 'http:' || u.protocol === 'https:') && u.host !== '';
}

interface LinkHint {
  show(event: MouseEvent, uri: string): void;
  hide(): void;
}

function createLinkHint(term: Terminal): LinkHint | null {
  const root = term.element;
  if (!root) return null;
  const el = document.createElement('div');
  // xterm-hover keeps the hint's own mouse events from activating links under it.
  el.className = `xterm-hover absolute ${tooltipContentVariants()}`;
  // Inline style: the tooltip's inline-flex would beat a `hidden` class.
  el.style.display = 'none';
  el.dataset.testid = 'terminal-link-hint';
  root.append(el);
  return {
    show(event, uri) {
      const box = root.getBoundingClientRect();
      el.textContent = `${uri}\n${t('workbench.terminal.link.tooltip', { modifier: primaryModifierName() })}`;
      el.style.display = '';
      const left = Math.min(event.clientX - box.left, Math.max(0, box.width - el.offsetWidth));
      el.style.left = `${Math.max(0, left)}px`;
      const below = event.clientY - box.top + 16;
      // Links sit near the bottom (above Claude Code's input), so flip above the pointer there.
      const top =
        below + el.offsetHeight > box.height
          ? event.clientY - box.top - el.offsetHeight - 8
          : below;
      el.style.top = `${Math.max(0, top)}px`;
    },
    hide() {
      el.style.display = 'none';
    },
  };
}

// Module-level, not held by the Vue component: a tab switch unmounts
// RepoTerminalView.vue (MainView.vue keeps only RepoGraphView alive), and the whole point of this
// map is that the live Terminal survives that — the sink stays attached, output keeps landing,
// and remounting re-inserts the same DOM node rather than recreating it.
const byTabId = new Map<string, Attached>();

// The close path never reaches this module (state/tabKinds.ts's dropResources only
// calls closeTerminalSession, which never imports this file — importing it there would pull
// @xterm/xterm into the boot bundle, the exact cost this file's own lazy boundary exists to
// avoid). registerTabRuntimeCleanup is the seam built for exactly this: registered once this
// module is loaded (i.e. once a terminal was opened), called for every tab that
// closes for real, a no-op for a tabId with no entry here — tabRuntime.ts's own documented shape
// ("a view kind whose module was never loaded has nothing registered here").
registerTabRuntimeCleanup((tabId) => {
  const attached = byTabId.get(tabId);
  if (!attached) return;
  byTabId.delete(tabId);
  attached.off();
  attached.term.dispose();
});

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

// §6.4: the four colours that have a token get one; the sixteen ANSI colours keep xterm's own
// defaults (the VS Code Dark+ palette this app's own tokens are already derived from) —
// deliberate, not an oversight (§6.4's own stated reasoning).
function terminalTheme(): import('@xterm/xterm').ITheme {
  return {
    background: cssVar('--kira-bg-chrome'),
    foreground: cssVar('--kira-fg'),
    cursor: cssVar('--kira-fg'),
    selectionBackground: cssVar('--kira-select'),
    scrollbarSliderBackground: cssVar('--kira-scrollbar'),
    scrollbarSliderHoverBackground: cssVar('--kira-scrollbar-hover'),
    scrollbarSliderActiveBackground: cssVar('--kira-scrollbar-active'),
  };
}

const encoder = new TextEncoder();

/** Returns tabId's own attached Terminal, creating and wiring it on first call. `host` is a
 *  detached `<div>` — RepoTerminalView.vue appends it to its container on mount and never calls
 *  `term.open()` a second time (the one mechanically safe way to reattach an xterm across
 *  unmounts, since whether `open()` itself is re-entrant is never relied on). */
export function getOrCreateTerminal(tabId: string, deps: TerminalRendererDeps): Attached {
  const existing = byTabId.get(tabId);
  if (existing) return existing;

  const host = document.createElement('div');
  host.classList.add('h-full', 'w-full');

  const appearance = deps.appearance();
  let hint: LinkHint | null = null;
  const activate = (event: MouseEvent, uri: string): void => {
    if (deps.linkActivation === 'modifier' && !isPrimaryModifier(event)) return;
    if (!isWebUrl(uri)) return;
    hint?.hide();
    deps.openLink(uri);
  };
  const linkHover = {
    hover: (e: MouseEvent, uri: string) => hint?.show(e, uri),
    leave: () => hint?.hide(),
  };
  const term = new Terminal({
    linkHandler: { activate, ...linkHover, allowNonHttpProtocols: false },
    scrollback: 5000,
    cursorBlink: true,
    allowProposedApi: false,
    // VS Code parity: keeps screens cleared with ED2 in scrollback.
    scrollOnEraseInDisplay: true,
    fontFamily: appearance.fontFamily,
    fontSize: appearance.fontSize,
    theme: terminalTheme(),
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.loadAddon(new WebLinksAddon(activate, linkHover));
  // Shift+Enter sends ESC CR, xterm's own Alt+Enter bytes: a newline in Claude Code and zsh.
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.key !== 'Enter' || !ev.shiftKey || ev.metaKey || ev.ctrlKey || ev.altKey) return true;
    if (ev.type === 'keydown' && !ev.isComposing) {
      ev.preventDefault();
      term.input('\x1b\r', true);
    }
    return false;
  });
  term.open(host);
  if (deps.linkActivation === 'modifier') hint = createLinkHint(term);
  const fileLinks = deps.fileLinks;
  if (fileLinks) {
    term.registerLinkProvider(
      createFileLinkProvider(term, tabId, fileLinks, {
        open: (event, target) => {
          if (deps.linkActivation === 'modifier' && !isPrimaryModifier(event)) return;
          hint?.hide();
          fileLinks.open(tabId, target);
        },
        hover: (event, text) => hint?.show(event, text),
        leave: () => hint?.hide(),
      }),
    );
  }

  const off = deps.onTerminalOutput(tabId, (bytes) => term.write(bytes));
  // onData is real text (typed characters, paste, escape sequences) — UTF-8 encoded before it
  // crosses the wire. onBinary is xterm's own byte-per-char convention (mouse reports, Alt-meta) —
  // each JS char code already IS the intended byte value, so it is never UTF-8 encoded, only
  // reinterpreted 1:1 into a Uint8Array. Getting these two swapped would corrupt typed non-ASCII
  // text (mis-encoding it) or corrupt binary escape sequences (double-UTF-8-encoding raw bytes).
  term.onData((d) => deps.writeTerminal(tabId, encoder.encode(d)));
  term.onBinary((d) => {
    const bytes = new Uint8Array(d.length);
    for (let i = 0; i < d.length; i++) bytes[i] = d.charCodeAt(i) & 0xff;
    deps.writeTerminal(tabId, bytes);
  });

  const attached: Attached = { term, fit, host, off };
  byTabId.set(tabId, attached);
  return attached;
}

/** Re-measures tabId's own terminal against its current container size and applies it — the
 *  caller (RepoTerminalView.vue's ResizeObserver) turns the result into a resizeTerminal call. A
 *  no-op, not an error, for an id with no attached terminal (the container observed before
 *  getOrCreateTerminal ever ran, or after this tab's own close). */
export function fitTerminal(tabId: string): { cols: number; rows: number } | null {
  const attached = byTabId.get(tabId);
  if (!attached) return null;
  attached.fit.fit();
  return { cols: attached.term.cols, rows: attached.term.rows };
}

/** The last `lines` non-empty-trailing rows of tabId's terminal, '' when it has none. */
export function tailTerminal(tabId: string, lines: number): string {
  const attached = byTabId.get(tabId);
  if (!attached) return '';
  const buf = attached.term.buffer.active;
  const out: string[] = [];
  for (let y = 0; y < buf.length; y++) out.push(buf.getLine(y)?.translateToString(true) ?? '');
  while (out.length > 0 && out[out.length - 1] === '') out.pop();
  return out.slice(-lines).join('\n');
}

/** §6.3: re-applies the live "Data font" setting to an already-open terminal and re-fits it —
 *  the settings watch's own target (RepoTerminalView.vue). A no-op for an id with no attached
 *  terminal. */
export function applyTerminalAppearance(
  tabId: string,
  deps: TerminalRendererDeps,
): { cols: number; rows: number } | null {
  const attached = byTabId.get(tabId);
  if (!attached) return null;
  const appearance = deps.appearance();
  attached.term.options.fontFamily = appearance.fontFamily;
  attached.term.options.fontSize = appearance.fontSize;
  return fitTerminal(tabId);
}
