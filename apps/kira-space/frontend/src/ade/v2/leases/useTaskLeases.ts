import { computed } from 'vue';
import type { LeaseEntry } from '../wire';
import { useLeases } from './queries';

export interface TaskLease {
  queue: string;
  /** 0 when the task holds the queue. */
  position: number;
}

/** One lease per task for its row: a held queue wins over a waiting one, else the best position. */
export function useTaskLeases() {
  const { data } = useLeases();
  const byTask = computed(() => {
    const out = new Map<string, TaskLease>();
    const offer = (e: LeaseEntry, queue: string): void => {
      const cur = out.get(e.taskId);
      if (!cur || e.position < cur.position) out.set(e.taskId, { queue, position: e.position });
    };
    for (const q of data.value?.queues ?? []) {
      if (q.holder) offer(q.holder, q.name);
      for (const w of q.waiters) offer(w, q.name);
    }
    return out;
  });
  return { byTask };
}
