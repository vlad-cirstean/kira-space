import type { RevealResult } from '@shared/domain/variables';
import { t } from '@theme/i18n';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';

// The one reveal loop the module's three call sites share —
// revealVariable and revealHistoryEntry (state/variables.ts) and the Copy as curl loop
// (state/curl.ts, which calls revealVariable rather than hand-rolling its own copy). The
// four-outcome switch and the recurse-once-on-confirmation shape were identical in all three;
// project/state/connectionReveal.ts keeps its own copy on purpose — sharing it would need a home
// both project/** and http/** may import, which is exactly the Studio<->Api coupling this module
// boundary exists to prevent.
// Returns this call's own outcome — the revealed plaintext on
// success, `undefined` for anything else (cancelled, still-unavailable, or errored) — rather than
// `void`. A caller that instead re-reads a shared reveal map after awaiting this can't tell a
// fresh cancellation from a stale, unrelated success sitting in that map from an earlier reveal of
// the same id; returning the outcome of *this* call closes that gap at the source.
export async function runReveal(
  call: (confirmed: boolean) => Promise<RevealResult>,
  onRevealed: (value: string) => void,
  onError: (message: string) => void,
  prompt: string,
): Promise<string | undefined> {
  const handle = async (result: RevealResult): Promise<string | undefined> => {
    switch (result.outcome) {
      case 'revealed':
        if (result.value === null) return undefined;
        onRevealed(result.value);
        return result.value;
      case 'cancelled':
        return undefined;
      case 'confirmation-required': {
        const ok = await useConfirmDialogStore().confirm({
          title: t('studio.api.reveal.revealConfirm.title'),
          message: prompt,
          confirmLabel: t('common.action.show.label'),
          tone: 'primary',
        });
        return ok ? await handle(await call(true)) : undefined;
      }
      default:
        onError(result.error ?? t('studio.api.reveal.failed.error'));
        return undefined;
    }
  };
  return await handle(await call(false));
}
