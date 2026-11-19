import { type MessageKey, t } from '@theme/i18n';
import type { WorktreeSetup } from '../wire';

const TITLE = {
  running: 'space.ade.board.setup.running.title',
  ready: 'space.ade.board.setup.ready.title',
  failed: 'space.ade.board.setup.failed.title',
} as const satisfies Record<WorktreeSetup['state'], MessageKey>;

/** The `ScriptProgress` props of one branch's setup. */
export function setupStatus(setup: WorktreeSetup, repo: string, branch: string, timeout: string) {
  return {
    state: setup.state,
    title: t('common.dotPair.label', {
      first: t(TITLE[setup.state]),
      second: `${repo} ${branch}`,
    }),
    startedAt: setup.startedAt,
    finishedAt: setup.finishedAt,
    timeout: timeout || undefined,
    note: setup.note || undefined,
  };
}
