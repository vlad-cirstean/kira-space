import { normalizeBoardEvent } from '@ade/boardEvent';
import type { AdeSignalSource } from '@ade/readSignals';
import type { BoardChangedEvent, LogEvent, RunsChangedEvent } from '@ade/wire';
import { CHANNEL } from '@shared/protocol/events';
import { useEventSource } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { computed, onScopeDispose, ref, watch } from 'vue';
import { getJson } from './http';

// Server-sent events from /api/events. Channels are module-level so read signals subscribe once at
// boot, before the stream opens, and survive reconnects.

const CHANNELS = [
  CHANNEL.adeTaskBoard,
  CHANNEL.adeTaskWorkflows,
  CHANNEL.adeTaskRepos,
  CHANNEL.adeTaskRuns,
  CHANNEL.adeTaskLog,
  CHANNEL.adeTaskSessions,
  CHANNEL.agentSessions,
  CHANNEL.agentEvent,
] as const;

type Listener = (data: unknown) => void;
const listeners = new Map<string, Set<Listener>>();

export function onChannel(channel: string, cb: Listener): () => void {
  let set = listeners.get(channel);
  if (!set) {
    set = new Set();
    listeners.set(channel, set);
  }
  set.add(cb);
  return () => set.delete(cb);
}

function dispatch(channel: string, raw: string): void {
  let data: unknown;
  try {
    data = JSON.parse(raw);
  } catch {
    return;
  }
  for (const cb of listeners.get(channel) ?? []) cb(data);
}

export const sseSignalSource: AdeSignalSource = {
  onBoard: (cb) =>
    onChannel(CHANNEL.adeTaskBoard, (d) => cb(normalizeBoardEvent(d as BoardChangedEvent | null))),
  onWorkflows: (cb) => onChannel(CHANNEL.adeTaskWorkflows, () => cb()),
  onSessions: (cb) => onChannel(CHANNEL.adeTaskSessions, () => cb()),
  onRuns: (cb) => onChannel(CHANNEL.adeTaskRuns, (d) => cb(d as RunsChangedEvent)),
  onLog: (cb) => onChannel(CHANNEL.adeTaskLog, (d) => cb(d as LogEvent)),
};

export type ConnectionStatus = 'live' | 'reconnecting' | 'offline';

/** Opens the stream for the calling scope and closes it with the scope. `onResync` fires after
 *  every reconnect: pushes sent while disconnected are gone, so callers refetch. */
export function useServerEvents(onResync: () => void) {
  const { eventSource, status, error } = useEventSource('/api/events', [], {
    autoReconnect: { retries: -1, delay: TIMING.reconnect },
  });
  const opened = ref(false);

  watch(
    eventSource,
    (es) => {
      if (!es) return;
      for (const channel of CHANNELS) {
        es.addEventListener(channel, (e) => dispatch(channel, (e as MessageEvent<string>).data));
      }
    },
    { immediate: true },
  );
  watch(status, (s) => {
    if (s !== 'OPEN') return;
    if (opened.value) onResync();
    opened.value = true;
  });
  // A dropped stream is a revoked device or a lost network; any 401 from this probe navigates to
  // the pairing screen through the shared handler.
  watch(error, (e) => {
    if (e)
      void getJson('/api/me').catch(() => {
        // Any 401 already navigated through the shared handler; other failures change nothing here.
      });
  });
  onScopeDispose(() => eventSource.value?.close());

  const connection = computed<ConnectionStatus>(() => {
    if (status.value === 'OPEN') return 'live';
    return status.value === 'CONNECTING' ? 'reconnecting' : 'offline';
  });
  return { connection };
}
