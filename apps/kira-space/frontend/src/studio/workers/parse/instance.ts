import { createParseClient } from './client';

/**
 * The app-wide parse worker client; the worker starts on the first `run`. The `new URL` form (not a
 * `?worker` import) keeps this module loadable under `bun test`, which cannot resolve `?worker`.
 */
export const parseClient = createParseClient(
  () => new Worker(new URL('./parse.worker.ts', import.meta.url), { type: 'module' }),
);
