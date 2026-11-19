import { type QueryClient, VueQueryPlugin } from '@tanstack/vue-query';
import { CONFIRM_DISCARD_KEY, type ConfirmDiscard } from '@theme/components/ui/dialog';
import { i18n } from '@theme/i18n';
import { type App, createApp } from 'vue';
import { errorText } from './bridge/codedError';
import { logError } from './bridge/log';
import BootFailure from './components/BootFailure.vue';
import { useConfirmDialogStore } from './state/confirmDialog';

// kira-space's and kira-studio's own main.ts each wrote this identical catch-and-retry wrapper
// around their own app-specific mountShell() — mountShell's own hydrates can reject (a DB error,
// or a busy DB past its lock timeout). Left uncaught, that rejection skips app.mount entirely: a
// permanently blank window, logged only as an unhandled rejection in the webview console. This
// mounts BootFailure instead, with a Retry that re-runs the whole sequence.
// Theme dialogs cannot import the confirm store, so the shell hands them a discard confirm built on
// it. Also installs `VueQueryPlugin`: `useAsyncAction` is a TanStack mutation, so every app that
// runs one (git-ui, Monaco view-zone apps included) needs a client. Call on the app before `mount`,
// after `app.use(pinia)`.
const confirmDiscard: ConfirmDiscard = ({ title, body, confirmLabel }) =>
  useConfirmDialogStore().confirm({ title, message: body, confirmLabel, tone: 'danger' });

export function provideShellServices(app: App, queryClient: QueryClient): void {
  app.use(VueQueryPlugin, { queryClient });
  app.provide(CONFIRM_DISCARD_KEY, confirmDiscard);
}

export async function bootstrapShell(
  mountShell: () => Promise<void>,
  appName: string,
): Promise<void> {
  try {
    await mountShell();
  } catch (err) {
    logError('bootstrap', 'failed to hydrate/mount the shell', err);
    const failureApp = createApp(BootFailure, {
      appName,
      message: errorText(err),
      onRetry: () => {
        failureApp.unmount();
        void bootstrapShell(mountShell, appName);
      },
    });
    failureApp.use(i18n);
    failureApp.mount('#app');
  }
}
