import { base64ToBytes } from '@shared/domain/base64';
import { canonicalPath } from '@shared/domain/path';
import type { TerminalLaunchKind } from '@shared/domain/tabs';
import type { TerminalEvent } from '@shared/protocol/events';
import { t } from '@theme/i18n';
import { promiseTimeout } from '@vueuse/core';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import { TIMING } from '../behaviour/timing';
import { errorText } from '../bridge/codedError';
import type { TabTerminalPayload, TerminalAdopt, TerminalHold } from '../bridge/createCoreControl';
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
  /** Tab move (Kira Space only; absent in Docker exec). */
  terminalHold?(terminalId: string): Promise<TerminalHold>;
  terminalAdopt?(terminalId: string): Promise<TerminalAdopt>;
  terminalRelease?(terminalId: string): Promise<void>;
}

/** What a terminal tab needs to be re-created in the window it moves to. */
export interface AdoptedTerminalTab {
  tabId: string;
  codeRepoId: string;
  cwd: string;
  command: string;
  launchKind: TerminalLaunchKind;
}

const encoder = new TextEncoder();

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
    if (__KIRA_DEBUG_HOOKS__) {
      (window as unknown as Record<string, unknown>).__kiraTerminalDrainBytes = () => {
        let total = 0;
        for (const d of drainByTabId.values()) total += d.bytes;
        return total;
      };
    }

    // Cumulative bytes routed per terminal (event `end`), so a move can wait for every event the
    // source was sent before it serializes; events and call responses do not order against each other.
    const routedEnd = new Map<string, number>();
    const routedWaiters = new Map<string, { end: number; done: () => void }[]>();
    // Ids whose output is queued, not written, until the Adopt answer says where the held bytes end.
    const adopting = new Map<string, { end: number; bytes: Uint8Array }[]>();
    // Source-window screens waiting for the new xterm's sink; written first, exempt from the drain cap.
    const snapshots = new Map<string, string>();

    function noteRouted(tabId: string, end: number): void {
      routedEnd.set(tabId, end);
      const waiters = routedWaiters.get(tabId);
      if (!waiters) return;
      const rest = waiters.filter((w) => {
        if (end < w.end) return true;
        w.done();
        return false;
      });
      if (rest.length > 0) routedWaiters.set(tabId, rest);
      else routedWaiters.delete(tabId);
    }

    function waitRouted(tabId: string, end: number, ms: number): Promise<void> {
      if ((routedEnd.get(tabId) ?? 0) >= end) return Promise.resolve();
      let waiter: { end: number; done: () => void } | undefined;
      const reached = new Promise<void>((done) => {
        waiter = { end, done };
        routedWaiters.set(tabId, [...(routedWaiters.get(tabId) ?? []), waiter]);
      });
      return Promise.race([reached, promiseTimeout(ms)]).finally(() => {
        const rest = (routedWaiters.get(tabId) ?? []).filter((w) => w !== waiter);
        if (rest.length > 0) routedWaiters.set(tabId, rest);
        else routedWaiters.delete(tabId);
      });
    }

    function appendToDrain(tabId: string, bytes: Uint8Array, bounded = true): void {
      let drain = drainByTabId.get(tabId);
      if (!drain) {
        drain = { chunks: [], bytes: 0 };
        drainByTabId.set(tabId, drain);
      }
      drain.chunks.push(bytes);
      drain.bytes += bytes.byteLength;
      while (bounded && drain.bytes > DRAIN_LIMIT_BYTES && drain.chunks.length > 1) {
        const dropped = drain.chunks.shift();
        if (dropped) drain.bytes -= dropped.byteLength;
      }
    }

    // Routes decoded output bytes to an attached sink, or the drain queue's own window.
    // Output arriving after closeTerminalSession has already deleted this tab's byTabId entry
    // (racing terminalClose) has nowhere left to drain to -- buffering it anyway just leaks up to
    // DRAIN_LIMIT_BYTES per stale tab id forever, so the drain branch checks byTabId first.
    function routeTerminalData(terminalId: string, bytes: Uint8Array, end = 0): void {
      const queue = adopting.get(terminalId);
      if (queue) {
        queue.push({ end, bytes });
        return;
      }
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
        // Docker exec events carry no `end`.
        const end = typeof event.end === 'number' ? event.end : undefined;
        if (event.data) routeTerminalData(event.terminalId, base64ToBytes(event.data), end);
        if (end !== undefined) noteRouted(event.terminalId, end);
        applyTerminalStatus(event);
      });
    }

    // A session opened with no view mounted (Kira Space's ADE opens them headlessly and shows one
    // only when the user later opens its pane) gets its xterm created now, so xterm's own parser
    // keeps terminal modes and its scrollback bounds memory, instead of a byte drain losing the
    // session's start-up sequences. The renderer stays a lazy chunk: the store never imports xterm.
    // Last size the pty was told, so a headless xterm starts at the real grid, not 80x24, and output
    // that arrives before a view mounts wraps as it will later.
    const lastSize = new Map<string, { cols: number; rows: number }>();

    async function attachHeadlessTerminal(
      tabId: string,
      launchKind: TerminalLaunchKind,
    ): Promise<void> {
      try {
        const renderer = await loadTerminalRenderer();
        if (!byTabId.has(tabId) || sinks.has(tabId)) return;
        renderer.getOrCreateTerminal(
          tabId,
          {
            appearance: options.appearance,
            onTerminalOutput,
            writeTerminal,
            openLink: (url) => void options.openExternal(url),
            linkActivation: 'modifier',
            fileLinks: options.fileLinks,
          },
          lastSize.get(tabId),
          launchKind,
        );
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
      lastSize.set(tabId, { cols, rows });
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
      if (!sinks.has(tabId)) void attachHeadlessTerminal(tabId, launchKind);

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
      lastSize.set(tabId, { cols, rows });
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
    function forget(tabId: string): void {
      byTabId.delete(tabId);
      lastSize.delete(tabId);
      sinks.delete(tabId);
      drainByTabId.delete(tabId);
      routedEnd.delete(tabId);
      adopting.delete(tabId);
      snapshots.delete(tabId);
      writer.drop(tabId);
    }

    function closeTerminalSession(tabId: string): void {
      if (!byTabId.has(tabId)) return;
      forget(tabId);
      fireAndForget(control.terminalClose(tabId), 'terminals');
    }

    /** The session moved to another window: drop every local trace without killing the pty. */
    function detachTerminalSession(tabId: string): void {
      forget(tabId);
    }

    /** Source side of a tab move: holds a live session's output, waits for every event already sent,
     *  then serializes the screen. Throws when the hold is refused; the caller releases on a later
     *  failure. */
    async function prepareTerminalMove(
      tabId: string,
      opts?: { maxBytes?: number },
    ): Promise<{ payload: TabTerminalPayload; live: boolean }> {
      const sess = byTabId.get(tabId);
      if (!sess) throw new Error('terminal has no session');
      let live = false;
      if (sess.status === 'running') {
        if (!control.terminalHold) throw new Error('terminal move is not supported');
        const held = await control.terminalHold(tabId);
        live = held.live;
        if (live) await waitRouted(tabId, held.end, TIMING.terminalMoveSettle);
      }
      const renderer = await loadTerminalRenderer();
      const shot = await renderer.serializeTerminal(tabId, opts?.maxBytes);
      const size = lastSize.get(tabId) ?? { cols: 80, rows: 24 };
      const status = live || sess.status === 'running' ? 'running' : sess.status;
      return {
        live,
        payload: {
          snapshot: shot?.snapshot ?? '',
          cols: shot?.cols ?? size.cols,
          rows: shot?.rows ?? size.rows,
          status: status === 'starting' ? 'running' : status,
          exitCode: sess.exitCode ?? undefined,
          error: sess.error ?? undefined,
          shell: sess.shell,
        },
      };
    }

    /** Ends a hold after a failed move: Go replays the held bytes here and output resumes. */
    async function releaseTerminalMove(tabId: string): Promise<void> {
      await control.terminalRelease?.(tabId);
    }

    // Writes what the Adopt answer held, then the live chunks that arrived meanwhile and are newer.
    function replayAdopted(tabId: string, result: TerminalAdopt): void {
      const deliver = (bytes: Uint8Array): void => {
        const sink = sinks.get(tabId);
        if (sink) sink(bytes);
        else appendToDrain(tabId, bytes, false);
      };
      if (result.truncated) {
        const notice = t('workbench.terminal.moveTruncated.status');
        deliver(encoder.encode(`\r\n\x1b[2m${notice}\x1b[0m\r\n`));
      }
      if (result.data) deliver(base64ToBytes(result.data));
      for (const chunk of adopting.get(tabId) ?? []) {
        if (chunk.end > result.end) deliver(chunk.bytes);
      }
      adopting.delete(tabId);
      if (result.exited) {
        applyTerminalStatus({
          terminalId: tabId,
          end: result.end,
          exited: true,
          exitCode: result.exitCode,
          error: result.error,
        });
      }
    }

    /** Target side of a tab move: re-creates the session entry, then (live) takes over the output. */
    async function adoptTerminalSession(
      tab: AdoptedTerminalTab,
      payload: TabTerminalPayload,
    ): Promise<void> {
      const { tabId } = tab;
      ensureSubscribed();
      lastSize.set(tabId, { cols: payload.cols, rows: payload.rows });
      byTabId.set(tabId, {
        tabId,
        codeRepoId: tab.codeRepoId,
        cwd: canonicalPath(tab.cwd),
        command: tab.command,
        status: payload.status,
        exitCode: payload.exitCode ?? null,
        error: payload.error ?? null,
        shell: payload.shell,
      });
      if (payload.snapshot) snapshots.set(tabId, payload.snapshot);
      if (payload.status !== 'running') {
        void attachHeadlessTerminal(tabId, tab.launchKind);
        return;
      }
      adopting.set(tabId, []);
      void attachHeadlessTerminal(tabId, tab.launchKind);
      try {
        if (!control.terminalAdopt) throw new Error('terminal move is not supported');
        replayAdopted(tabId, await control.terminalAdopt(tabId));
      } catch (err) {
        adopting.delete(tabId);
        logError('terminals', `${tabId}: adopt failed`, err);
        const sess = byTabId.get(tabId);
        if (sess) {
          sess.status = 'failed';
          sess.error = errorText(err);
        }
      }
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

      const snapshot = snapshots.get(tabId);
      if (snapshot !== undefined) {
        snapshots.delete(tabId);
        sink(encoder.encode(snapshot));
      }
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
      detachTerminalSession,
      prepareTerminalMove,
      releaseTerminalMove,
      adoptTerminalSession,
      terminalSession,
      terminalCountAtPath,
      onTerminalOutput,
    };
  });
}
