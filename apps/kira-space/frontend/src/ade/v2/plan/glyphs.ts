import { ICON } from '@theme/icons';
import type {
  BranchAction,
  BranchActionKind,
  BranchTagKind,
  TaskActionKind,
  TaskTagKind,
} from '../board/actions';

// Codicon per tag state and action. Desktop-only: actions.ts is shared with mobile.
export interface Glyph {
  icon: string;
  /** Louder look: a solid disc. */
  urgent?: boolean;
  /** Spins as a `Spinner`; the icon is `loading` or `sync`. */
  rotating?: boolean;
}

export const TASK_TAG_GLYPH: Record<TaskTagKind, Glyph> = {
  status: { icon: 'circle-filled' },
  blocked: { icon: 'error', urgent: true },
  merged: { icon: 'git-merge' },
  notMerging: { icon: 'circle-slash' },
};

export const BRANCH_TAG_GLYPH: Record<BranchTagKind, Glyph> = {
  notCreated: { icon: 'circle-large-outline' },
  preparing: { icon: ICON.settings },
  setupFailed: { icon: 'error', urgent: true },
  merged: { icon: 'git-merge' },
  parked: { icon: 'circle-slash' },
  review: { icon: 'eye' },
  reviewConflict: { icon: 'flame', urgent: true },
  conflict: { icon: 'flame', urgent: true },
  rebaseConflicts: { icon: 'flame', urgent: true },
  rebasing: { icon: 'sync', rotating: true, urgent: true },
  rebasePending: { icon: 'history', urgent: true },
  rebaseFailed: { icon: 'sync-ignored', urgent: true },
  rebaseInProgress: { icon: 'debug-pause', urgent: true },
  baseMissing: { icon: 'debug-disconnect', urgent: true },
  notPushed: { icon: 'cloud-upload' },
  behind: { icon: 'arrow-down' },
  sharesFiles: { icon: 'files' },
  waiting: { icon: 'watch' },
  baseAttention: { icon: 'warning' },
  checking: { icon: 'loading', rotating: true },
  checkFailed: { icon: 'question' },
  clean: { icon: 'check' },
};

export const TASK_ACTION_ICON: Record<TaskActionKind, string> = {
  run: 'play',
  session: 'debug-start',
  resume: 'debug-continue',
  advance: 'arrow-right',
  finish: 'pass',
  takeOver: 'person',
  approve: 'check',
  retry: 'debug-restart',
  archive: 'archive',
};

export const BRANCH_ACTION_ICON: Record<BranchActionKind, string> = {
  forcePush: 'repo-force-push',
  rebase: 'repo-sync',
  queue: 'list-ordered',
  changeBase: 'git-branch',
  abortRebase: 'debug-stop',
  seeLog: 'output',
  takeOver: 'person',
  seeError: 'bug',
  start: 'debug-start',
};

/** A row's status cluster holds at most 3 icons: tag + 2 actions, or tag + 1 action + More. */
export function rowActionSplit(actions: readonly BranchAction[]): {
  shown: BranchAction[];
  overflow: BranchAction[];
} {
  return actions.length <= 2
    ? { shown: [...actions], overflow: [] }
    : { shown: actions.slice(0, 1), overflow: actions.slice(1) };
}
