import { errorText } from '@workbench/bridge/codedError';
import { handlers } from './handlers';
import type { JobKind, WorkerRequest, WorkerResponse } from './protocol';

// The frontend tsconfig has no `WebWorker` lib; type only the two members used.
const ctx = self as unknown as {
  postMessage: (m: WorkerResponse) => void;
  addEventListener: (type: 'message', cb: (e: MessageEvent<WorkerRequest>) => void) => void;
};

ctx.addEventListener('message', async (e) => {
  const { id, kind, input } = e.data;
  try {
    const run = handlers[kind as JobKind] as (i: unknown) => unknown;
    ctx.postMessage({ id, ok: true, output: await run(input) });
  } catch (err) {
    ctx.postMessage({ id, ok: false, error: errorText(err) });
  }
});
