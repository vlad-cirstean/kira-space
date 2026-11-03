import { base64ToBytes } from '@shared/domain/base64';
import { canonicalPath } from '@shared/domain/path';
import type { TerminalLaunchKind } from '@shared/domain/tabs';
import type { TerminalEvent } from '@shared/protocol/events';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { errorText } from '../bridge/codedError';
import { fireAndForget } from '../bridge/fireAndForget';
import { logError } from '../bridge/log';
import type { TerminalFileLinks } from '../terminal/terminalRenderer';
import { loadTerminalRenderer } from '../terminal/terminalRendererLoader';
import { createOrderedWriter } from './orderedWrites';

// Hoisted from both apps' own state/terminals.ts — byte-identical (the Kira Studio Automations
// module, the Kira Space repo-worktree terminal), once each app's own `canonicalPath` resolved to
// the same packages/shared function (this phase found Kira Space's state/coderepos.ts still
// defining a local duplicate — folded into @shared/domain/path, its documented single-definition
// intent). Parameterized only over `control` (the bound-call subset both apps' bridge/index.ts
// already expose with identical signatures) — no other divergence exists between the two apps for
// this store.

export interface TerminalSession {
  readonly tabId: string;
  readonly codeRepoId: string;
  readonly cwd: string; // absolute, already canonical (§8.1)
  readonly command: string; // '' means a plain login shell (P83's own behaviour) — P85 §5.4
  status: 'starting' | 'running' | 'exited' | 'failed';
  exitCode: number | null;
  error: string | null;
  shell: string;
}

// Output that arrives before a sink attaches is queued here: the window between
// openTerminalSession and the xterm renderer chunk resolving (a headless session, see
// attachHeadlessTerminal), never the long wait for a view — xterm itself buffers from then on.
// Bounded per tab so a pathological case cannot grow without limit — oldest dropped past
// DRAIN_LIMIT_BYTES, but the newest chunk is always kept.
const DRAIN_LIMIT_BYTES = 256 * 1024;
interface Drain {
  chunks: Uint8Array[];
  bytes: number;
}

function bytesToBase64(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin);
}

/** The bound-call subset this store needs — both apps' bridge/index.ts already implement it with
 *  identical signatures (TerminalService's own Wails-generated bindings). */
export interface TerminalsControl {
  terminalDefaultCwd(): Promise<{ path: string }>;
  onTerminal(cb: (event: TerminalEvent) => void): () => void;
  terminalOpen(
    terminalId: string,
    cwd: string,
    cols: number,
    rows: number,
    command?: string,
    launchKind?: TerminalLaunchKind,
    scriptId?: string,
    launchToken?: string,
  ): Promise<{ shell: string }>;
  terminalWrite(terminalId: string, data: string): Promise<void>;
  terminalResize(terminalId: string, cols: number, rows: number): Promise<void>;
  terminalClose(terminalId: string): Promise<void>;
}

// P99: was module-level reactive()/plain-Map state. Now a Pinia setup store, same
// single-responsibility module.
export interface TerminalsStoreOptions {
  /** The live "Data font" setting, read when a headless session's xterm is created. */
  appearance(): { fontFamily: string; fontSize: number };
  /** Opens a terminal link in the OS browser, read when a headless session's xterm is created. */
  openExternal(url: string): Promise<void>;
  /** Pinia store id; a second terminals store (Docker exec) needs its own. Default `terminals`. */
  storeId?: string;
  /** File-path links; set only where Kira Space can open them. */
  fileLinks?: TerminalFileLinks;
}

export function createTerminalsStore(control: TerminalsControl, options: TerminalsStoreOptions) {
  return defineStore(options.storeId ?? 'terminals', () => {
    // The Automations module's own unscoped-launch default — the user's home directory,
    // resolved in Go (bridge/terminal.go's DefaultCwd) and hydrated once at boot, beside
    // hydrateCustomScripts (main.ts). '' means "not yet hydrated, or $HOME could not be resolved" —
    // every caller (TabStrip.vue's Terminal entry, AutomationsStart.vue's button) disables its
    // launch on that value rather than falling back to some other path.
    const terminalDefaults = reactive({ cwd: '' });

    async function hydrateTerminalDefaults(): Promise<void> {
      const { path } = await control.terminalDefaultCwd();
      terminalDefaults.cwd = path;
    }

    // reactive() on the Map itself (not a plain Map), for repo/state/worktrees.ts's own recorded
    // reason: the panel reads a key before any entry exists, and a plain Map makes that read
    // untracked.
    const byTabId = reactive(new Map<string, TerminalSession>());

    const sinks = new Map<string, (bytes: Uint8Array) => void>();
    const drainByTabId = new Map<string, Drain>();

    function appendToDrain(tabId: string, bytes: Uint8Array): void {
      let drain = drainByTabId.get(tabId);
      if (!drain) {
        drain = { chunks: [], bytes: 0 };
        drainByTabId.set(tabId, drain);
      }
      drain.chunks.push(bytes);
      drain.bytes += bytes.byteLength;
      while (drain.bytes > DRAIN_LIMIT_BYTES && drain.chunks.length > 1) {
        const dropped = drain.chunks.shift();
        if (dropped) drain.bytes -= dropped.byteLength;
      }
    }

    // Routes decoded output bytes to an attached sink, or the drain queue's own window.
    // Output arriving after closeTerminalSession has already deleted this tab's byTabId entry
    // (racing terminalClose) has nowhere left to drain to -- buffering it anyway just leaks up to
    // DRAIN_LIMIT_BYTES per stale tab id forever, so the drain branch checks byTabId first.
    function routeTerminalData(terminalId: string, bytes: Uint8Array): void {
      const sink = sinks.get(terminalId);
      if (sink) {
        sink(bytes);
      } else if (byTabId.has(terminalId)) {
        appendToDrain(terminalId, bytes);
      }
    }

    function applyTerminalStatus(event: TerminalEvent): void {
      const sess = byTabId.get(event.terminalId);
      if (!sess) return;
      if (event.exited) {
        sess.status = event.error ? 'failed' : 'exited';
        sess.exitCode = event.exitCode ?? null;
        sess.error = event.error || null;
      } else if (sess.status === 'starting') {
        sess.status = 'running';
      }
    }

    // One module-level control.onTerminal(...) subscription, installed on first use, routes by
    // terminalId (§5.2): a sink attached (the normal case) gets the decoded bytes directly; no sink
    // yet (the drain queue's own window) appends instead.
    let subscribed = false;
    function ensureSubscribed(): void {
      if (subscribed) return;
      subscribed = true;
      control.onTerminal((event) => {
        if (event.data) routeTerminalData(event.terminalId, base64ToBytes(event.data));
        applyTerminalStatus(event);
      });
    }

    // A session opened with no view mounted (Kira Space's ADE opens them headlessly and shows one
    // only when the user later opens its pane) gets its xterm created now, so xterm's own parser
    // keeps terminal modes and its scrollback bounds memory, instead of a byte drain losing the
    // session's start-up sequences. The renderer stays a lazy chunk: the store never imports xterm.
    async function attachHeadlessTerminal(tabId: string): Promise<void> {
      try {
        const renderer = await loadTerminalRenderer();
        if (!byTabId.has(tabId) || sinks.has(tabId)) return;
        renderer.getOrCreateTerminal(tabId, {
          appearance: options.appearance,
          onTerminalOutput,
          writeTerminal,
          openLink: (url) => void options.openExternal(url),
          linkActivation: 'modifier',
          fileLinks: options.fileLinks,
        });
      } catch (err) {
        logError('terminals', `${tabId}: renderer load failed`, err);
      }
    }

    /** Subscribes, then calls TerminalService.Open — terminalId is the tab id, client-supplied, so
     *  ensureSubscribed runs (synchronously) before the bound call ever reaches Go, and no output can
     *  race the subscription (§3.2/§5.2). cwd is canonicalized once here, so every other read in this
     *  module (terminalCountAtPath, the tab's own cwd) compares like with like. P86 §4: launchKind is
     *  forwarded to TerminalOpenArgs.LaunchKind as-is — Go, not this module, decides what a
     *  'agent' launch gets (hooks, the agent count). */
    async function openTerminalSession(
      tabId: string,
      codeRepoId: string,
      cwd: string,
      cols: number,
      rows: number,
      command = '',
      launchKind: TerminalLaunchKind = 'shell',
      scriptId = '',
      launchToken = '',
    ): Promise<void> {
      ensureSubscribed();
      const cwdCanonical = canonicalPath(cwd);
      byTabId.set(tabId, {
        tabId,
        codeRepoId,
        cwd: cwdCanonical,
        command,
        status: 'starting',
        exitCode: null,
        error: null,
        shell: '',
      });
      if (!sinks.has(tabId)) void attachHeadlessTerminal(tabId);

      try {
        const { shell } = await control.terminalOpen(
          tabId,
          cwdCanonical,
          cols,
          rows,
          command,
          launchKind,
          scriptId,
          launchToken,
        );
        const sess = byTabId.get(tabId);
        if (sess) sess.shell = shell;
      } catch (err) {
        const sess = byTabId.get(tabId);
        if (sess) {
          sess.status = 'failed';
          sess.error = errorText(err);
        }
      }
    }

    // Concurrent bound calls arrive in any order, so keystrokes go one write at a time per tab.
    const writer = createOrderedWriter(
      (tabId, bytes) => control.terminalWrite(tabId, bytesToBase64(bytes)),
      (tabId, err) => logError('terminals', `${tabId}: write failed`, err),
    );

    /** A pass-through, not an encoding decision: bytes is already the exact byte sequence to send —
     *  UTF-8 for typed text, a raw Latin1-per-char passthrough for xterm's own onBinary (mouse
     *  reports, Alt-meta) — terminalRenderer.ts picks which, since only it knows which callback the
     *  data came from. base64-encoded here purely because that's the wire format the bound call
     *  takes (keystrokes are not always valid UTF-8). */
    function writeTerminal(tabId: string, bytes: Uint8Array): void {
      writer.write(tabId, bytes);
    }

    function resizeTerminal(tabId: string, cols: number, rows: number): void {
      fireAndForget(control.terminalResize(tabId, cols, rows), 'terminals');
    }

    /** dropResources' own target (state/tabKinds.ts) — kills the pty and drops every trace of this
     *  tab's session, including anything still queued in the drain. `dropPageStoresForTab`
     *  blind-calls
     *  every registered kind's own dropper on every tab close (state/tabs.ts), so a non-terminal
     *  tab
     *  id reaches this too — the no-op-miss guard below is what makes that safe, the same contract
     *  every other kind's dropResources (e.g. views/repo/editors.ts's dropRepoFileTab)
     *  already follows.
     *  Without it, closing any tab of any kind fired a needless TerminalService.Close round trip.
     */
    function closeTerminalSession(tabId: string): void {
      if (!byTabId.has(tabId)) return;
      byTabId.delete(tabId);
      sinks.delete(tabId);
      drainByTabId.delete(tabId);
      writer.drop(tabId);
      fireAndForget(control.terminalClose(tabId), 'terminals');
    }

    function terminalSession(tabId: string): TerminalSession | undefined {
      return byTabId.get(tabId);
    }

    /** The whole signal — the count of live (not yet exited/failed) sessions whose cwd matches
     *  path, canonicalized the same way openTerminalSession's own cwd is. Excluding exited/failed
     *  sessions is what makes a shell exiting on its own repaint the row (the "opening or
     *  exiting"), even though the tab itself stays open and its entry stays in this map. */
    function terminalCountAtPath(path: string): number {
      const target = canonicalPath(path);
      let count = 0;
      for (const sess of byTabId.values()) {
        if (sess.cwd === target && sess.status !== 'exited' && sess.status !== 'failed') count++;
      }
      return count;
    }

    /** views/repo/terminalRenderer.ts's own subscription (§6.2): attaches sink as tabId's output
     *  handler, draining anything §5.2's queue already holds for it. Returns the detach function. */
    function onTerminalOutput(tabId: string, sink: (bytes: Uint8Array) => void): () => void {
      ensureSubscribed();
      sinks.set(tabId, sink);

      const drained = drainByTabId.get(tabId);
      if (drained) {
        drainByTabId.delete(tabId);
        for (const chunk of drained.chunks) sink(chunk);
      }

      return () => {
        if (sinks.get(tabId) === sink) sinks.delete(tabId);
      };
    }

    return {
      terminalDefaults,
      byTabId,
      hydrateTerminalDefaults,
      openTerminalSession,
      writeTerminal,
      resizeTerminal,
      closeTerminalSession,
      terminalSession,
      terminalCountAtPath,
      onTerminalOutput,
    };
  });
}
