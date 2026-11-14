import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { errorText } from '../bridge/codedError';

export interface ConfirmOptions {
  message: string;
  /** Dialog title, a question naming the action. */
  title: string;
  confirmLabel: string;
  /** `danger` discards or deletes (confirm button red, focus starts on Cancel). Always stated. */
  tone: 'danger' | 'primary';
  /** Work to do on confirm. The dialog stays open with a busy button and shows a rejection inline;
   *  it closes when `run` resolves. */
  run?: () => Promise<unknown>;
}

export type ConfirmChoice = 'confirm' | 'secondary' | 'cancel';

export interface ChooseOptions {
  title: string;
  message: string;
  confirmLabel: string;
  /** Third button between Cancel and confirm (for example Discard beside Save). */
  secondaryLabel: string;
  tone: 'danger' | 'primary';
}

// Replaces window.confirm() for every destructive action in the app. A native confirm panel
// blocks the renderer, steals window focus (closing any open context menu) and is not reliably
// auto-acceptable by Playwright; an ordinary Teleported HTML dialog has none of those problems.
export const useConfirmDialogStore = defineStore('confirmDialog', () => {
  const state = reactive({
    open: false,
    title: '',
    message: '',
    tone: 'danger' as 'danger' | 'primary',
    confirmLabel: '',
    secondaryLabel: '',
    busy: false,
    error: '',
    run: null as (() => Promise<unknown>) | null,
    resolve: null as ((choice: ConfirmChoice) => void) | null,
  });

  function open(
    options: ConfirmOptions & { secondaryLabel?: string },
    resolve: (choice: ConfirmChoice) => void,
  ): void {
    // F9: a second call before the first settles would otherwise overwrite `state.resolve`,
    // leaving the first caller's promise pending forever. Settle it (as a decline) first.
    state.resolve?.('cancel');
    state.title = options.title;
    state.message = options.message;
    state.tone = options.tone;
    state.confirmLabel = options.confirmLabel;
    state.secondaryLabel = options.secondaryLabel ?? '';
    state.run = options.run ?? null;
    state.busy = false;
    state.error = '';
    state.resolve = resolve;
    state.open = true;
  }

  async function confirm(options: ConfirmOptions): Promise<boolean> {
    return (await new Promise<ConfirmChoice>((resolve) => open(options, resolve))) === 'confirm';
  }

  function choose(options: ChooseOptions): Promise<ConfirmChoice> {
    return new Promise((resolve) => open(options, resolve));
  }

  function finish(choice: ConfirmChoice): void {
    state.resolve?.(choice);
    state.open = false;
    state.resolve = null;
    state.run = null;
  }

  async function acceptConfirm(): Promise<void> {
    if (state.busy) return;
    if (!state.run) {
      finish('confirm');
      return;
    }
    state.busy = true;
    state.error = '';
    try {
      await state.run();
      finish('confirm');
    } catch (err) {
      state.error = errorText(err);
    } finally {
      state.busy = false;
    }
  }

  function cancelConfirm(): void {
    if (state.busy) return;
    finish('cancel');
  }

  function acceptSecondary(): void {
    if (state.busy) return;
    finish('secondary');
  }

  return { ...toRefs(state), confirm, choose, acceptConfirm, acceptSecondary, cancelConfirm };
});
