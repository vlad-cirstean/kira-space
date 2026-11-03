import { VueQueryPlugin } from '@tanstack/vue-query';
import { i18n, installCopyTestHook } from '@theme/i18n';
import { bootstrapShell, provideShellServices } from '@workbench/bootstrapShell';
import { logError } from '@workbench/bridge/log';
import { windowKey } from '@workbench/lib/window';
import { queryClient } from '@workbench/state/queryClient';
import { createApp } from 'vue';
import App from './App.vue';
import { installAdeSignals } from './ade/queries';
import { useAgentSessionsStore } from './ade/state';
import { reviewTargetKey } from './ade/v2/queries';
import { adeReaderKey } from './ade/v2/reader';
import { useAdeReviewWindowStore } from './ade/v2/state/adeReviewWindow';
import { controlAdeReader } from './bridge/adeReader';
import { control } from './bridge/control';
import { captureItemKey } from './capture/queries';
import { useCaptureWindowStore } from './capture/state';
import { hostViewTargetKey } from './hostview/queries';
import { useHostViewWindowStore } from './hostview/state';
import { gitWindowReady } from './repo/git/boundGit';
import { useAdeLayoutStore } from './state/adeLayout';
import { installAgentNotifyFocus } from './state/agentNotify';
import { useAppMetricsStore } from './state/appMetrics';
import { useAppUpdateStore } from './state/appUpdate';
import { useCodeReposStore } from './state/coderepos';
import { useCustomScriptsStore } from './state/customScripts';
import { useGitCredentialStore } from './state/gitCredential';
import { useKeepAwakeStore } from './state/keepAwake';
import { useLayoutStore } from './state/layout';
import { useMobileAccessStore } from './state/mobileAccess';
import { useMobileTerminalsStore } from './state/mobileTerminals';
import { useModeStore } from './state/mode';
import { openPendingFiles } from './state/openedFiles';
import { useOpsStore } from './state/ops';
import { pinia } from './state/pinia';
import { ensureWorkspaceShell } from './state/repoTabs';
import { useSettingsStore } from './state/settings';
import { useTabsStore } from './state/tabs';
import { useTerminalsStore } from './state/terminals';
import { GENERAL_WORKSPACE, useWorkspaceStore } from './state/workspace';
// styles.css re-exports @workbench/workbench.css (itself @theme/base.css) and adds one more @source
// over packages/git-ui/src, so this app's own unprefixed root scans git-ui's migrated shadcn call
// sites too. Importing both here would compile base.css as two separate Tailwind roots and double
// its output — this is the one entry point now.
import './styles.css';

// Kira Studio's own main.ts bootstrap, trimmed to this app's own state layer — no
// __KIRA_DEBUG_HOOKS__ block (that whole retention-probe apparatus is data-grid/query-result
// specific: grid/documents/keyvalue/stream/console page stores, none of which exist here) and no
// ops/dbMcp stores (none of those subsystems exist in this app — apps/kira-space/
// main.go's own Services list has no counterpart for any of them). Studio's own
// agentHooks/agentSessions stores left too (agent-activity monitoring moved to a shared home, used
// by no app as of this phase), so that gap has since closed on its own.
// This app now has its own metrics ticker and
// keep-awake toggle (main.go's own metrics.NewAppTicker/keepawake.New).
// This app now has its own update checker/installer.
async function mountShell(): Promise<void> {
  // Every store used here runs before app.use(pinia) below, so each needs the module-level
  // `pinia` instance passed explicitly (Pinia has no active instance yet at this point).
  const agentSessionsStore = useAgentSessionsStore(pinia);
  const opsStore = useOpsStore(pinia);
  const appMetricsStore = useAppMetricsStore(pinia);
  const keepAwakeStore = useKeepAwakeStore(pinia);
  const layoutStore = useLayoutStore(pinia);
  const adeLayoutStore = useAdeLayoutStore(pinia);
  const modeStore = useModeStore(pinia);
  const settingsStore = useSettingsStore(pinia);
  const codeReposStore = useCodeReposStore(pinia);
  const customScriptsStore = useCustomScriptsStore(pinia);
  const gitCredentialStore = useGitCredentialStore(pinia);
  const mobileAccessStore = useMobileAccessStore(pinia);
  const mobileTerminalsStore = useMobileTerminalsStore(pinia);
  const tabsStore = useTabsStore(pinia);
  const terminalsStore = useTerminalsStore(pinia);
  const workspaceStore = useWorkspaceStore(pinia);
  const reviewWindowStore = useAdeReviewWindowStore(pinia);
  const hostViewStore = useHostViewWindowStore(pinia);
  const captureStore = useCaptureWindowStore(pinia);

  // Right after the stores are built, before mount() below, so no push (`kira:adetask:*`
  // channels/agent `Stop`) is missed between mount and this window's first
  // `useAdeSnapshot`/`useAdeSessions` call. No teardown — the window is the lifetime
  // (queries.ts's own doc comment).
  installAdeSignals(queryClient, agentSessionsStore);

  // Resets this window's git connection once per page load, ahead of the first git call.
  gitWindowReady().catch((err: unknown) => logError('git', 'window reset failed', err));

  // Kira Studio's own initAppMetrics precedent: just subscribes, no data dependency — runs
  // synchronously before the Promise.all below rather than joining it.
  appMetricsStore.initAppMetrics();

  // This app now persists a per-window module mode too (internal/windowsvc,
  // shared with Kira Studio) — hydrated before any other window-scoped state,
  // mirroring Kira Studio's own main.ts:338. The fall-forward onto the first restored repo below
  // still runs regardless of which mode this resolves to (state/workspace.ts's own
  // `openRepoWorkspace`/`activateWorkspace` doc comment: that fall-forward must honour a persisted
  // `terminal`/`ade` mode, not silently override it back to `git`).
  modeStore.hydrateMode(await control.windowsEnsure());

  await Promise.all([
    layoutStore.hydrateLayout(),
    adeLayoutStore.hydrate(),
    settingsStore.hydrateSettings(),
    codeReposStore.initCodeRepos(),
    tabsStore.hydrateTabs(),
  ]);

  // P116 G5: keep-awake hydrate joins the optional group below, not this critical Promise.all — a
  // stuck/erroring OS power-assertion call must never block this app's own boot the way it's
  // allowed to gate Kira Studio's (that app's own main.ts keeps it in the critical group).

  // F2: git credential prompts, terminals (new-terminal defaults) and the rest are not on
  // the critical path to a rendered shell — Promise.allSettled so one of these hitting a DB error
  // never takes down the whole window the way it did bundled into the Promise.all above.
  const optional = await Promise.allSettled([
    gitCredentialStore.hydrateRelayPrompts(),
    mobileAccessStore.hydrateMobileAccess(),
    mobileTerminalsStore.hydrateMobileTerminals(),
    terminalsStore.hydrateTerminalDefaults(),
    customScriptsStore.hydrateCustomScripts(),
    keepAwakeStore.initKeepAwake(),
    // P129 Part 3 §2.8 item 1: the boot-time hydrate for the ade module's own agent-activity store
    // (repo tabs' needs-input badge, §0.15) — a stuck/erroring subscribe must never block the rest
    // of this app's own boot, same reasoning as every other member of this group.
    agentSessionsStore.initAgentSessions(),
    // P132 Part 2: the op log dock's snapshot. A failed hydrate still goes live (createOpLogStore).
    opsStore.hydrateOps(),
  ]);
  for (const result of optional) {
    if (result.status === 'rejected') {
      logError('bootstrap', 'optional store hydrate failed', result.reason);
    }
  }

  // A capture popup (ephemeral, one clipboard item) answers with its item; any other gets null.
  const captureItem = await queryClient.query({
    queryKey: captureItemKey(windowKey),
    queryFn: () => control.captureWindowItem({ windowKey }),
  });
  if (captureItem) captureStore.item = captureItem;

  // A host view window (ephemeral, one change request) answers with its target; any other gets null.
  const hostViewTarget = await queryClient.query({
    queryKey: hostViewTargetKey(windowKey),
    queryFn: () => control.hostViewWindowTarget({ windowKey }),
  });
  if (hostViewTarget) hostViewStore.target = hostViewTarget;

  // A review window (ephemeral, one branch) answers with its target; any other window gets null.
  const reviewTarget = await queryClient.query({
    queryKey: reviewTargetKey(windowKey),
    queryFn: () => control.adeTaskReviewWindowTarget({ windowKey }),
  });
  if (reviewTarget) {
    reviewWindowStore.target = reviewTarget;
    workspaceStore.openReviewWorkspace(reviewTarget.codeRepoId);
  }

  // hydrateTabs (state/tabs.ts) already derived workspaceStore.openRepos from the restored tabs
  // themselves — ensureWorkspaceShell's own doc comment calls for running it once per restored repo
  // right here, after hydrateTabs, since hydrateTabs itself never calls back into it (avoiding a
  // third link in that module pair's existing two-way call graph).
  const workbenchWindow = !reviewTarget && !hostViewTarget && !captureItem;
  if (workbenchWindow) for (const repoId of workspaceStore.openRepos) ensureWorkspaceShell(repoId);
  // Falls forward onto the last active repo (persisted per window), else the first restored one,
  // so a relaunch with open repositories skips the empty GitStart screen. `activateWorkspace`
  // never forces the mode back to `git`, so the mode `windowsEnsure` restored stays.
  if (
    workbenchWindow &&
    workspaceStore.active === GENERAL_WORKSPACE &&
    workspaceStore.openRepos.length > 0
  ) {
    const last = workspaceStore.lastRepoKey;
    workspaceStore.activateWorkspace(
      last !== null && workspaceStore.openRepos.includes(last)
        ? last
        : (workspaceStore.openRepos[0] as string),
    );
  }

  // Files Finder opened into this window; after the fall-forward so the last one wins focus.
  if (workbenchWindow) await openPendingFiles();

  const app = createApp(App);
  app.use(i18n);
  app.provide(adeReaderKey, controlAdeReader);
  app.use(pinia);
  provideShellServices(app);
  app.use(VueQueryPlugin, { queryClient });
  app.mount('#app');
  // Review windows show no agent sessions, so only a normal window reports focus.
  if (workbenchWindow) installAgentNotifyFocus();
  // Off the boot critical path — Kira Studio's own main.ts precedent (an update check gains
  // nothing from blocking first paint, and Go's own cache floor decides what actually fetches).
  useAppUpdateStore().initAppUpdate();
}

// mountShell's own essential hydrates (layout/settings/codeRepos/tabs) can still reject — a DB
// error, or a busy DB past the 5s _busy_timeout (the second-instance case makes this plausible).
// The catch-and-retry wrapper itself moved to workbench/bootstrapShell.ts, shared with
// kira-studio's own identical copy.
// Test builds only: the copy-coverage specs switch the locale to the pseudo-locale.
if (__KIRA_DEBUG_HOOKS__) installCopyTestHook();

void bootstrapShell(mountShell, 'Kira Space');
