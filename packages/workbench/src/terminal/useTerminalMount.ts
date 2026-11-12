import type { TerminalLaunchKind, TerminalTabState } from '@shared/domain/tabs';
import { t } from '@theme/i18n';
import { computed, onMounted, onUnmounted, type Ref, watch } from 'vue';
import type { TerminalRendererDeps } from './terminalRenderer';
import { loadTerminalRenderer } from './terminalRendererLoader';

export type TerminalMountTabState = Pick<
  TerminalTabState,
  'codeRepoId' | 'cwd' | 'command' | 'launchKind'
> & { scriptId?: string; launchToken?: string };

/** The subset of createTerminalsStore.ts's own TerminalSession this needs — status/exitCode/error
 *  drive footerText below, nothing else. */
export interface TerminalMountSession {
  status: 'starting' | 'running' | 'exited' | 'failed';
  exitCode: number | null;
  error: string | null;
}

export interface UseTerminalMountOptions {
  tabId: string;
  /** Read once, at the tab's very first mount — never watched (this component is keyed by tab.id,
   *  so its identity never changes across this instance's lifetime; a session, once open, is never
   *  reopened for the same tab, §7.3/§7.4 below). */
  tabState: TerminalMountTabState;
  /** The caller's own template ref (`<div ref="container" />`) — kept caller-owned, not created
   *  and returned here, so each view's own `container` binding stays a real script reference (not
   *  a `noUnusedVariables` false positive: biome's linter has no Vue-template awareness of a
   *  `ref="container"` binding, only of `container` being read somewhere in `<script>`). */
  container: Ref<HTMLElement | null>;
  rendererDeps: TerminalRendererDeps;
  terminalSession: (tabId: string) => TerminalMountSession | undefined;
  openTerminalSession: (
    tabId: string,
    codeRepoId: string,
    cwd: string,
    cols: number,
    rows: number,
    command: string,
    launchKind: TerminalLaunchKind,
    scriptId: string,
    launchToken: string,
  ) => Promise<void>;
  resizeTerminal: (tabId: string, cols: number, rows: number) => void;
}

/** TerminalView.vue (Kira Studio) and RepoTerminalView.vue (Kira Space) — a duplicate,
 *  not an import (the two frontends are separate Vite apps with no shared package boundary; see
 *  TerminalView.vue's own header comment for why this whole area is "duplicate, don't
 *  hoist", same call as the Monaco bootstrap and Go's internal/terminal package). This owns the
 *  one piece genuinely identical between the two views: the mount/resize/appearance-watch/unmount
 *  lifecycle around terminalRenderer.ts. Each view keeps its own template and whatever renders
 *  around `container` (Kira Studio's Claude Code hooks banner; Kira Space has none). */
export function useTerminalMount(options: UseTerminalMountOptions) {
  const {
    tabId,
    tabState,
    container,
    rendererDeps,
    terminalSession,
    openTerminalSession,
    resizeTerminal,
  } = options;

  let resizeObserver: ResizeObserver | null = null;
  let renderer: typeof import('./terminalRenderer') | null = null;
  let pendingFocus = false;

  const session = computed(() => terminalSession(tabId));
  const footerText = computed(() => {
    const s = session.value;
    if (!s) return '';
    if (s.status === 'failed') return s.error || t('workbench.terminal.startFailed.error');
    if (s.status === 'exited')
      return t('workbench.terminal.exited.status', { code: s.exitCode ?? 0 });
    return '';
  });

  async function mount(): Promise<void> {
    const mod = await loadTerminalRenderer();
    // Unmounted (tab closed/switched away) while the import above was in flight.
    if (!container.value) return;
    renderer = mod;

    const { host } = mod.getOrCreateTerminal(tabId, rendererDeps);
    container.value.appendChild(host);

    // §7.3/§7.4: openTerminalSession only on the tab's very first mount — a remount (switching
    // back to an already-open terminal) reattaches the same live session, never spawns a second one.
    const isFirstOpen = !terminalSession(tabId);
    const dims = mod.fitTerminal(tabId) ?? { cols: 80, rows: 24 };
    if (pendingFocus) {
      pendingFocus = false;
      mod.focusTerminal(tabId);
    }
    if (isFirstOpen) {
      void openTerminalSession(
        tabId,
        tabState.codeRepoId,
        tabState.cwd,
        dims.cols,
        dims.rows,
        tabState.command,
        tabState.launchKind,
        tabState.scriptId ?? '',
        tabState.launchToken ?? '',
      );
    }

    // No debounce needed — a ResizeObserver callback already coalesces synchronous layout thrash
    // into one notification per frame (SlickGridHost.vue's own precedent).
    // not useResizeObserver — this construction sits after `await loadTerminalRenderer()`
    // above, past the point Vue's synchronous "current instance" tracking a composable's automatic
    // onUnmounted registration relies on; moving it earlier would mean guarding every callback
    // invocation against `renderer` still being null, a real behavior change for a mechanical
    // conversion. Declined, named per CLAUDE.md's library rule.
    resizeObserver = new ResizeObserver(() => {
      const d = mod.fitTerminal(tabId);
      if (d) resizeTerminal(tabId, d.cols, d.rows);
    });
    resizeObserver.observe(container.value);
  }

  onMounted(() => void mount());

  // A mere tab switch away: the ResizeObserver stops (nothing to measure while hidden), and the
  // host detaches with its parent — the live Terminal and its Go-side pty both survive (§6.2/§6.3).
  // Only a real close tears them down (each app's own dropResources, terminal-kind entry).
  onUnmounted(() => {
    resizeObserver?.disconnect();
    resizeObserver = null;
  });

  // §6.3: the Appearance "Data font" setting is read live, not only at mount — a change while this
  // terminal is open re-applies immediately.
  watch(
    () => {
      const a = rendererDeps.appearance();
      return [a.fontFamily, a.fontSize] as const;
    },
    () => {
      const d = renderer?.applyTerminalAppearance(tabId, rendererDeps);
      if (d) resizeTerminal(tabId, d.cols, d.rows);
    },
  );

  /** Gives the terminal the keyboard; a call before the renderer is ready applies once it is. */
  function focusTerminal(): void {
    if (renderer) renderer.focusTerminal(tabId);
    else pendingFocus = true;
  }

  return { session, footerText, focusTerminal };
}
