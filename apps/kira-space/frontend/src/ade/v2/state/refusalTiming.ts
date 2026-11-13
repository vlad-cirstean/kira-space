import { TIMING } from '@workbench/behaviour/timing';

/**
 * Wraps an op's call to time its last run. `prompt()` is true when that run ended before the slow-op
 * mark: the user is still looking, so a refusal may reopen its dialog. Time spent at a repo prompt is not
 * counted, only the bridge call itself.
 */
export function timedCall<T>(call: () => Promise<T>): {
  call: () => Promise<T>;
  prompt: () => boolean;
} {
  let took = 0;
  return {
    call: async () => {
      const began = Date.now();
      try {
        return await call();
      } finally {
        took = Date.now() - began;
      }
    },
    prompt: () => took < TIMING.slowOp,
  };
}
