// Demo bytes for the phone terminal shot: an agent session on the story task, written for the
// shot. Not a recording; names no product. Lines stay under 40 columns so no wrap depends on fit.

const BOLD = '\x1b[1m';
const DIM = '\x1b[2m';
const GREEN = '\x1b[32m';
const CYAN = '\x1b[36m';
const YELLOW = '\x1b[33m';
const RESET = '\x1b[0m';

const LINES = [
  `${BOLD}${CYAN}Implement${RESET} ${DIM}Export orders as CSV${RESET}`,
  '',
  `${GREEN}●${RESET} Read src/orders/exportCsv.ts`,
  `${DIM}  58 lines${RESET}`,
  '',
  `${GREEN}●${RESET} Edit src/orders/exportCsv.ts`,
  `${DIM}  added escapeCell for quotes${RESET}`,
  `${DIM}  newline is now \\r\\n${RESET}`,
  '',
  `${GREEN}●${RESET} Run bun test src/orders`,
  `${DIM}  orders/exportCsv.test.ts${RESET}`,
  `${GREEN}  12 pass${RESET}`,
  `${DIM}  0 fail${RESET}`,
  '',
  `${YELLOW}Export tests pass.${RESET}`,
  'Commit on feat/orders-csv-export',
  'and open the pull request?',
  '',
  `${BOLD}>${RESET} `,
];

/** Terminal bytes the replay sends; the last sequence hides the blinking cursor. */
export const PHONE_TERMINAL_BYTES = `${LINES.join('\r\n')}\x1b[?25l`;
