import type { ProofChartViewState, ProofViewState } from '@shared/domain/proof';
import { useDebounceFn, useThrottleFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { onBeforeUnmount, shallowRef, watch } from 'vue';
import { control } from '../bridge/control';
import { EMPTY_RESULT_VIEW_STATE, type ResultViewState } from '../views/console/resultViewState';
import { cacheViewState, useProofViewStates } from './queries';

const CACHE_THROTTLE_MS = 120;

function sameRecord(a: Record<string, unknown>, b: Record<string, unknown>): boolean {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((k) => a[k] === b[k]);
}

function sameState(a: ProofViewState, b: ProofViewState): boolean {
  return (
    a.queryScrollTop === b.queryScrollTop &&
    a.resultScrollTop === b.resultScrollTop &&
    a.resultScrollLeft === b.resultScrollLeft &&
    a.selectedRow === b.selectedRow &&
    JSON.stringify(a.selection) === JSON.stringify(b.selection) &&
    JSON.stringify(a.chart ?? null) === JSON.stringify(b.chart ?? null) &&
    a.expanded.length === b.expanded.length &&
    a.expanded.every((id, i) => id === b.expanded[i]) &&
    sameRecord(a.columnWidths, b.columnWidths)
  );
}

interface StepIds {
  sessionId: () => string;
  rev: () => number;
  ordinal: () => number;
}

/** One step's saved view: loaded once as the restore seed, then kept in the query cache on every
 *  change and written to the backend after a quiet moment (and on unmount). */
export function useStepViewState(ids: StepIds) {
  const query = useProofViewStates(ids.sessionId, ids.rev);
  const seed = shallowRef<ProofViewState | null>(null);
  let latest: ProofViewState | null = null;
  let saved = '';

  watch(
    () => query.data.value,
    (entries) => {
      if (!entries || seed.value) return;
      const state = entries.find((e) => e.ordinal === ids.ordinal())?.state;
      seed.value = state ?? { queryScrollTop: 0, ...EMPTY_RESULT_VIEW_STATE };
      latest = seed.value;
      saved = JSON.stringify(latest);
    },
    { immediate: true },
  );

  const sessionId = ids.sessionId();
  const rev = ids.rev();
  const ordinal = ids.ordinal();

  const write = useDebounceFn(() => flush(), TIMING.scrollPersist);
  // The cache write notifies every step view of the revision, so scroll bursts coalesce.
  const writeCache = useThrottleFn(() => {
    if (latest) cacheViewState(sessionId, rev, ordinal, latest);
  }, CACHE_THROTTLE_MS);

  function flush(): void {
    if (!latest) return;
    cacheViewState(sessionId, rev, ordinal, latest);
    const json = JSON.stringify(latest);
    if (json === saved) return;
    saved = json;
    fireAndForget(control.proofSetViewState(sessionId, rev, ordinal, latest), 'proof.viewState');
  }

  function update(patch: Partial<ProofViewState>): void {
    if (!latest) return;
    const next = { ...latest, ...patch };
    if (sameState(next, latest)) return;
    latest = next;
    void writeCache();
    void write();
  }

  onBeforeUnmount(flush);

  return {
    /** `null` until the saved view has loaded. */
    seed,
    loadFailed: query.isError,
    setResult: (state: ResultViewState) => update(state),
    setQueryScroll: (queryScrollTop: number) => update({ queryScrollTop }),
    setChart: (chart: ProofChartViewState) => update({ chart }),
  };
}
