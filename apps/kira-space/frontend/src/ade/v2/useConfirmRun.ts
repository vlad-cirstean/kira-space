import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { watch } from 'vue';

/**
 * The run of a state-driven ADE confirm dialog. `run` resolves with an error message to keep the
 * dialog open, or `null` on success; a rejection also stays open. `confirm` closes through `onDone`.
 */
export function useConfirmRun(
  open: () => boolean,
  run: () => Promise<string | null>,
  onDone: () => void,
) {
  const action = useAsyncAction(async () => {
    const message = await run();
    if (message !== null) throw new Error(message);
  });
  watch(open, (o) => {
    if (o) action.reset();
  });
  async function confirm(): Promise<void> {
    if (action.pending.value) return;
    if (await action.run()) onDone();
  }
  return { busy: action.pending, error: action.error, confirm };
}
