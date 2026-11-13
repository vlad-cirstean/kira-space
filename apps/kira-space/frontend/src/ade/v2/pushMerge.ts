import type { Board, LogChunk, LogPage, RunsChangedEvent } from './wire';

/** Merges pushed runs into the cached board by id (unknown ones append to their task); the board
 *  push that follows reconciles. */
export function mergeRuns(board: Board, event: RunsChangedEvent): Board {
  const tasks = board.tasks.map((t) => {
    const incoming = event.runs.filter((r) => r.taskId === t.id);
    if (!incoming.length) return t;
    const byId = new Map(incoming.map((r) => [r.id, r]));
    const runs = t.runs.map((r) => byId.get(r.id) ?? r);
    const known = new Set(t.runs.map((r) => r.id));
    for (const r of incoming) if (!known.has(r.id)) runs.push(r);
    return { ...t, runs };
  });
  return { ...board, tasks };
}

/** Text a cached log page keeps; a longer run drops its oldest chunks, so a chatty run cannot grow
 *  the cache and the rendered list without bound. */
export const LOG_RETAIN_CHARS = 2_000_000;

/** Appends pushed chunks to a cached log page, skipping sequence numbers it already holds. */
export function appendChunks(page: LogPage, chunks: LogPage['chunks']): LogPage {
  const last = page.chunks.at(-1)?.seq ?? page.nextSeq - 1;
  const fresh = chunks.filter((c) => c.seq > last);
  if (!fresh.length) return page;
  let merged = [...page.chunks, ...fresh];
  let truncated = page.truncated;
  let size = merged.reduce((n, c) => n + c.text.length, 0);
  let drop = 0;
  // The newest chunk always stays, however large.
  while (size > LOG_RETAIN_CHARS && drop < merged.length - 1) {
    size -= (merged[drop] as LogChunk).text.length;
    drop++;
  }
  if (drop > 0) {
    merged = merged.slice(drop);
    truncated = true;
  }
  return { ...page, chunks: merged, truncated, nextSeq: (fresh.at(-1)?.seq ?? last) + 1 };
}
