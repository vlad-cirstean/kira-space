import { handlers, type SyncKind } from './handlers';
import type { JobInput, JobKind, JobOutput, WorkerRequest, WorkerResponse } from './protocol';

/** Inputs up to this many chars run inline: a worker round trip would only add a pending flash. */
export const INLINE_CHARS = 65_536;

export interface WorkerLike {
  postMessage(message: WorkerRequest): void;
  terminate(): void;
  addEventListener(type: 'message', cb: (e: MessageEvent<WorkerResponse>) => void): void;
  addEventListener(type: 'error', cb: (e: unknown) => void): void;
}

export interface RunOptions {
  signal?: AbortSignal;
}

interface Job {
  id: number;
  kind: JobKind;
  input: unknown;
  signal?: AbortSignal;
  resolve: (v: never) => void;
  reject: (e: unknown) => void;
  aborted: boolean;
}

const abortError = (): DOMException => new DOMException('Aborted', 'AbortError');

/** Same handler, same tick. Only for kinds with a synchronous handler. */
export function parseInline<K extends SyncKind>(kind: K, input: JobInput<K>): JobOutput<K> {
  return (handlers[kind] as (i: JobInput<K>) => JobOutput<K>)(input);
}

export interface ParseClient {
  run<K extends JobKind>(kind: K, input: JobInput<K>, opts?: RunOptions): Promise<JobOutput<K>>;
}

/**
 * One lazily started worker, one job in flight, FIFO queue. A worker failure never fails a job:
 * pending jobs run inline through the same handlers and the next job spawns a fresh worker.
 */
export function createParseClient(makeWorker: () => WorkerLike): ParseClient {
  let worker: WorkerLike | null = null;
  let running: Job | null = null;
  const queue: Job[] = [];
  let nextId = 0;

  function settle(job: Job, run: () => unknown): void {
    Promise.resolve()
      .then(() => (job.aborted ? undefined : run()))
      .then(
        (v) => {
          if (!job.aborted) job.resolve(v as never);
        },
        (e) => {
          if (!job.aborted) job.reject(e);
        },
      );
  }

  function runInline(job: Job): void {
    settle(job, () => (handlers[job.kind] as (i: unknown) => unknown)(job.input));
  }

  function killWorker(): void {
    worker?.terminate();
    worker = null;
  }

  function onWorkerFailure(): void {
    killWorker();
    const inflight = running;
    running = null;
    const pending = inflight ? [inflight, ...queue.splice(0)] : queue.splice(0);
    for (const job of pending) if (!job.aborted) runInline(job);
  }

  function spawn(): WorkerLike {
    const w = makeWorker();
    w.addEventListener('message', (e) => {
      if (w !== worker) return;
      const job = running;
      if (!job || e.data.id !== job.id) return;
      running = null;
      const res = e.data;
      if (!job.aborted) {
        if (res.ok) job.resolve(res.output as never);
        else job.reject(new Error(res.error));
      }
      pump();
    });
    w.addEventListener('error', () => {
      if (w === worker) onWorkerFailure();
    });
    return w;
  }

  function pump(): void {
    if (running?.aborted && queue.length > 0) {
      // A sync parse cannot observe a message: terminating is the only hard cancel.
      killWorker();
      running = null;
    }
    if (running) return;
    const job = queue.shift();
    if (!job) return;
    try {
      worker ??= spawn();
      running = job;
      worker.postMessage({ id: job.id, kind: job.kind, input: job.input });
    } catch {
      killWorker();
      running = null;
      runInline(job);
      pump();
    }
  }

  function abort(job: Job): void {
    if (job.aborted) return;
    job.aborted = true;
    const at = queue.indexOf(job);
    if (at !== -1) queue.splice(at, 1);
    job.reject(abortError());
    if (running === job) pump();
  }

  return {
    run(kind, input, opts) {
      const signal = opts?.signal;
      if (signal?.aborted) return Promise.reject(abortError());
      return new Promise((resolve, reject) => {
        const job: Job = {
          id: nextId++,
          kind,
          input,
          signal,
          resolve: resolve as Job['resolve'],
          reject,
          aborted: false,
        };
        if (signal) {
          const onAbort = () => abort(job);
          signal.addEventListener('abort', onAbort, { once: true });
          const [res, rej] = [job.resolve, job.reject];
          job.resolve = (v) => {
            signal.removeEventListener('abort', onAbort);
            res(v);
          };
          job.reject = (e) => {
            signal.removeEventListener('abort', onAbort);
            rej(e);
          };
        }
        queue.push(job);
        pump();
      });
    },
  };
}
