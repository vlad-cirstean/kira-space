import { useMutation, useQuery } from '@tanstack/vue-query';
import { defineStore } from 'pinia';
import { computed, type InjectionKey, ref, watch } from 'vue';
import { useMutationAction } from '../behaviour/asyncAction';
import { usePersistedState } from '../behaviour/persisted';
import { TIMING } from '../behaviour/timing';
import { errorText, toCodedError } from '../bridge/codedError';
import type { AppUpdateStatus } from '../bridge/createCoreControl';
import { fireAndForget } from '../bridge/fireAndForget';
import { queryClient } from './queryClient';

// P119: hoisted from Kira Studio's own state/appUpdate.ts (the poll) and rebuilt onto
// createAppMetricsStore.ts's own factory shape, plus install/cancel and the dialog's own
// auto-open/dismiss rules — Kira Space's own copy of this file is the same call, `'Kira Space'`.

export interface AppUpdateControl {
  updateStatus(): Promise<AppUpdateStatus>;
  /** Forced check: bypasses the Go cache and rejects when the check fails. */
  updateCheck(): Promise<AppUpdateStatus>;
  updateInstall(): Promise<void>;
  updateCancelInstall(): Promise<void>;
  /** P246: asks the prompt router to show the update popup in one window. */
  promptsRaiseUpdate(ref: string): Promise<void>;
  promptsDismiss(id: string): Promise<void>;
}

// appMetrics.ts's own shape, pull instead of push: Go decides cadence (an hourly poll here is a
// ceiling, not the real interval — UpdateService.Status's own cache floor decides whether any
// given call actually reaches the network).

export function createAppUpdateStore(control: AppUpdateControl, appName: string) {
  return defineStore('appUpdate', () => {
    // Off until initAppUpdate() flips it (main.ts, after app.mount) — an update check gains
    // nothing from blocking first paint, and starting the query eagerly would fetch before then.
    const enabled = ref(false);

    const statusQuery = useQuery(
      {
        queryKey: ['appUpdate'],
        queryFn: control.updateStatus,
        refetchInterval: TIMING.pollHourly,
        // Today's setInterval poll ran regardless of window focus — kept, not tightened.
        refetchIntervalInBackground: true,
        staleTime: 0,
        enabled,
      },
      queryClient,
    );

    // A failed poll keeps the previous data and renders nothing — P66's "failure is silence" holds
    // (TanStack Query's own default: an error never clears `data`).
    const available = computed(() => statusQuery.data.value?.updateAvailable ?? false);
    const currentVersion = computed(() => statusQuery.data.value?.currentVersion ?? '');
    // Target is the one minor-stepped update on offer; newest is the highest published release.
    const targetVersion = computed(() => statusQuery.data.value?.targetVersion ?? '');
    const newestVersion = computed(() => statusQuery.data.value?.newestVersion ?? '');
    const stepped = computed(
      () =>
        available.value &&
        newestVersion.value !== '' &&
        newestVersion.value !== targetVersion.value,
    );
    const installLogPath = computed(() => statusQuery.data.value?.installLogPath ?? '');

    // Shared by every window of this app (same origin) — VueUse syncs it across windows via the
    // `storage` event, so **Later** in one window silences the dialog in every other one too.
    const dismissedVersion = usePersistedState('kira.appUpdate.dismissedVersion', '');

    const dialogOpen = ref(false);
    const installMutation = useMutation(
      { mutationKey: ['appUpdate', 'install'], mutationFn: control.updateInstall },
      queryClient,
    );
    // A cancel is the person's own choice, not a failure.
    const installAction = useMutationAction(installMutation, {
      describe: (err) => (toCodedError(err).code === 'E_CANCELLED' ? null : errorText(err)),
    });
    const installing = installAction.pending;
    const installError = installAction.error;

    // A menu/palette check: opens the dialog to show progress, the result or the failure.
    const checkRequested = ref(false);
    const checkMutation = useMutation(
      {
        mutationKey: ['appUpdate', 'check'],
        mutationFn: control.updateCheck,
        onSuccess: (s) => queryClient.setQueryData(['appUpdate'], s),
      },
      queryClient,
    );
    const checkAction = useMutationAction(checkMutation);
    const checking = checkAction.pending;
    const checkError = checkAction.error;

    // Net: the router shows the update popup once per new target, in one window; **Later**
    // silences that target until a different one appears. Skipped while installing — the dialog stays
    // open through its own installing state even if another window's poll lands in between.
    // `dialogOpen` is only the status-bar item's own local open.
    watch([available, targetVersion, dismissedVersion], ([isAvailable, target, dismissed]) => {
      if (installing.value) return;
      // The open dialog already shows the update (a manual check); no second popup.
      if (isAvailable && target !== dismissed && !dialogOpen.value) {
        // Best effort: the status-bar item still opens the dialog.
        fireAndForget(control.promptsRaiseUpdate(target), 'appUpdate');
      } else if (dismissed === target) {
        dialogOpen.value = false;
      }
    });

    function initAppUpdate(): void {
      enabled.value = true;
    }

    // The status-bar item's own click — ignores dismissal, always reopens.
    function openUpdateDialog(): void {
      checkAction.reset();
      dialogOpen.value = true;
    }

    async function checkForUpdates(): Promise<void> {
      if (installing.value) return;
      checkRequested.value = true;
      dialogOpen.value = true;
      await checkAction.run();
    }

    function later(): void {
      if (installing.value) return;
      // A closed dialog drops its check outcome, so a later open shows the current state.
      checkAction.reset();
      if (!available.value) {
        dialogOpen.value = false;
        checkRequested.value = false;
        return;
      }
      dismissedVersion.value = targetVersion.value;
      dialogOpen.value = false;
      // The popup is already hidden here; a stale one in another window closes with its own Later.
      fireAndForget(control.promptsDismiss(`update:${targetVersion.value}`), 'appUpdate');
    }

    async function install(): Promise<void> {
      // A successful InstallUpdate quits this process shortly after replying — nothing left to
      // update here; the window is going away.
      await installAction.run();
    }

    function cancelInstall(): void {
      fireAndForget(control.updateCancelInstall(), 'appUpdate');
    }

    return {
      appName,
      available,
      currentVersion,
      targetVersion,
      newestVersion,
      stepped,
      installLogPath,
      dialogOpen,
      installing,
      installError,
      checkRequested,
      checking,
      checkError,
      initAppUpdate,
      checkForUpdates,
      openUpdateDialog,
      later,
      install,
      cancelInstall,
    };
  });
}

export type AppUpdateStore = ReturnType<ReturnType<typeof createAppUpdateStore>>;

/** App.vue provides its update store here for the routed update popup (prompts/UpdatePrompt.vue). */
export const appUpdateStoreKey: InjectionKey<AppUpdateStore> = Symbol('appUpdateStore');
