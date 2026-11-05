import { SPINNER_DELAY } from '@theme/timing';

/** Every debounce, poll, tick and feedback duration in app code, in ms. One table so the same kind
 *  of wait is the same number everywhere. */
export const TIMING = {
  /** Search and filter input. */
  search: 150,
  /** Parse and validate previews of pasted or typed text. */
  preview: 250,
  /** UI-state writes. */
  persist: 150,
  /** Notes autosave. */
  autosave: 600,
  /** Whole tab-state serialisation (large payload, so slower than `persist`). */
  tabsSave: 1000,
  /** "Copied" feedback on the copy control. */
  copiedFeedback: 1500,
  /** First-letter type-ahead buffer reset. */
  typeahead: 500,
  /** Live-region clear-then-set gap. */
  reannounce: 100,
  /** How long the status line stays after a review or comment is posted. */
  postedStatus: 6000,
  /** Wait after agent activity before re-reading the files it may have written. */
  fileSettle: 300,
  /** Stat poll of the files a review window lists. */
  reviewStamp: 1500,
  /** Hover info popup delay. */
  hoverInfo: 400,
  pollFast: 5000,
  pollSlow: 60000,
  /** Mirrors internal/localauth.GraceWindow: the reveal gate's own grace is fixed, not sliding, so a
   *  long editing session cannot hold one authentication open. Keep in sync by hand; a Go
   *  time.Duration has no shared constant. A revealed plaintext re-masks once it expires. */
  revealGrace: 5 * 60 * 1000,
  /** Hourly refresh (update checks, slow caches). */
  pollHourly: 3_600_000,
  /** Elapsed-time display tick of a running operation. */
  runTick: 200,
  /** Wait before a dropped connection retries. */
  reconnect: 3000,
  /** Loading placeholders show only after this wait, so fast loads do not flash. */
  spinnerDelay: SPINNER_DELAY,
  /** Scroll-position persistence. */
  scrollPersist: 300,
  /** How long an agent paste waits for the terminal to settle. */
  agentPasteWait: 10000,
  /** Settle time for window focus changes before the server is told. */
  focusSync: 1000,
  clockTick: 1000,
  clockTickSlow: 30000,
} as const;
