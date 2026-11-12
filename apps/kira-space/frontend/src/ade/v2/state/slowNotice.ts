import { useTimeoutFn } from '@vueuse/core';

export interface SlowNoticeSettled {
  ok: boolean;
  error?: string;
  /** True when the loading notice was already shown. */
  shown: boolean;
}

export interface SlowNotice {
  /** Ends the op; a second call is ignored. */
  settle(ok: boolean, error?: string): void;
}

/**
 * Timer against settle for slow-only notices: `show` fires once `delayMs` passes, `onSettle` fires once,
 * with whether `show` ran. A settle at the same tick as the timer sees whichever ran first.
 */
export function slowNotice(opts: {
  delayMs: number;
  show: () => void;
  onSettle: (result: SlowNoticeSettled) => void;
}): SlowNotice {
  let shown = false;
  let done = false;
  const timer = useTimeoutFn(
    () => {
      if (done) return;
      shown = true;
      opts.show();
    },
    opts.delayMs,
    { immediate: false },
  );
  timer.start();
  return {
    settle(ok, error) {
      if (done) return;
      done = true;
      timer.stop();
      opts.onSettle({ ok, error, shown });
    },
  };
}
