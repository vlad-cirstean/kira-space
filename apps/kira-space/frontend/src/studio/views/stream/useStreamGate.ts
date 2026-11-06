import { t } from '@theme/i18n';
import { computed, ref, watch } from 'vue';
import { useConnectionsStore } from '../../state/connections';
import type { StreamTabRecord } from '../../state/tabDomain';
import { getPage, pageVersion, streamRow } from './page';
import { useStreamViewStore } from './state';

function visibilityLabel(seconds: number): string {
  if (seconds % 3600 === 0) return t('studio.stream.hours.status', { count: seconds / 3600 });
  if (seconds % 60 === 0) return t('studio.stream.minutes.status', { count: seconds / 60 });
  return t('studio.stream.seconds.status', { count: seconds });
}

export function redriveSentence(limit: number | 'none' | null): string {
  if (limit === null) return t('studio.stream.redriveUnknown.description');
  if (limit === 'none') return t('studio.stream.redriveNone.description');
  return t('studio.stream.redriveLimit.description', { count: limit });
}

/** Capability and read-only gates plus the loaded page for one stream tab. */
export function useStreamGate(tab: () => StreamTabRecord) {
  const connectionsStore = useConnectionsStore();
  const streamViewStore = useStreamViewStore();

  const caps = computed(() => {
    const connectionId = tab().connectionId;
    return connectionId ? (connectionsStore.states[connectionId]?.caps ?? null) : null;
  });
  const connRecord = computed(() => connectionsStore.connectionRecord(tab().connectionId));
  const rt = computed(() => streamViewStore.runtime[tab().id]);
  const running = computed(() => rt.value?.status === 'loading');
  const page = computed(() => {
    void pageVersion.n;
    return getPage(tab().id);
  });

  // SQS 'batch' pagination never auto-loads: every poll consumes messages, so only Poll reads.
  const isBatch = computed(() => caps.value?.pagination === 'batch');
  // The UI reads Caps, not connection.kind, to decide what to offer; kind only picks wording.
  const isKafka = computed(() => connRecord.value?.kind === 'kafka');
  // Caps are static per adapter; the connection's read-only flag narrows them here.
  // Shown page outlived its connection: kept, read-only, behind StaleDataStrip.
  const stale = computed(() => streamViewStore.isStale(tab().id));
  // Caps clear on disconnect; the last live ones keep Send/Delete visible (disabled) behind the strip.
  const lastCaps = ref(caps.value);
  watch(caps, (c) => {
    if (c) lastCaps.value = c;
  });
  const shownCaps = computed(() => caps.value ?? lastCaps.value);
  const canInsertCaps = computed(
    () => (shownCaps.value?.canInsert ?? false) && !connRecord.value?.readOnly,
  );
  const canDeleteCaps = computed(
    () => (shownCaps.value?.canDelete ?? false) && !connRecord.value?.readOnly,
  );
  const canInsert = computed(() => canInsertCaps.value && !stale.value);
  const canDelete = computed(() => canDeleteCaps.value && !stale.value);
  const isReadOnlyBatch = computed(() => isBatch.value && (connRecord.value?.readOnly ?? false));

  const redriveLimit = computed(() => page.value?.maxReceiveCount ?? null);
  const hideDuration = computed(() => visibilityLabel(tab().state.sqsVisibilityTimeoutSeconds));

  function rowAt(i: number) {
    void pageVersion.n;
    return streamRow(tab().id, i);
  }

  return {
    caps,
    connRecord,
    rt,
    running,
    page,
    isBatch,
    isKafka,
    stale,
    canInsertCaps,
    canDeleteCaps,
    canInsert,
    canDelete,
    isReadOnlyBatch,
    redriveLimit,
    hideDuration,
    rowAt,
  };
}
