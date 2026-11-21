import type { InjectionKey } from 'vue';

/** The wording of the discard confirm; the dialog supplies it so the host stays copy-free. */
export interface DiscardPrompt {
  title: string;
  body: string;
  confirmLabel: string;
}

/** Asks the user to confirm discarding unsaved edits; resolves true to discard. The workbench
 *  shell provides it from the confirm store (theme cannot import the store). */
export type ConfirmDiscard = (prompt: DiscardPrompt) => Promise<boolean>;

export const CONFIRM_DISCARD_KEY: InjectionKey<ConfirmDiscard> = Symbol('confirmDiscard');

/** Provided by `DialogContent` to `DialogHeader`: close through the dirty guard. */
/** Provided by `DialogContent`: whether it is a prompt (the X then shows the prompt hide chord). */
export const DIALOG_PROMPT_KEY: InjectionKey<boolean> = Symbol('dialogPrompt');

export const DIALOG_REQUEST_CLOSE_KEY: InjectionKey<() => Promise<void>> =
  Symbol('dialogRequestClose');
