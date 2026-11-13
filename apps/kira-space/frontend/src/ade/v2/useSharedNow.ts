import { createSharedComposable, useIntervalFn, useNow } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';

/** One 30 s clock for every row that shows a relative time, instead of a timer per row. */
export const useSharedNow = createSharedComposable(() =>
  useNow({ scheduler: (cb) => useIntervalFn(cb, TIMING.clockTickSlow) }),
);
