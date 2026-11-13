import { useAgentInfo } from '@workbench/agent/agentInfo';
import { computed } from 'vue';
import { usePlanModel } from '../plan/usePlanModel';
import type { ArchivedTask, Session } from '../wire';
import { reuseView, type SessionLookup, type SessionView, sessionView } from './sessionView';

/** `view(session)` for any session, resolved against the live board, workflows and repo names.
 *  `archived` supplies the task and branches the board no longer lists. */
export function useSessionViews(archived?: () => ArchivedTask | undefined) {
  const { model, sessions, repoLabel } = usePlanModel();
  const { info } = useAgentInfo();

  const lookup = computed<SessionLookup>(() => {
    const m = model.value;
    const tasks = new Map(m?.board.tasks.map((t) => [t.id, t]));
    const branches = m?.view.graph.byBranch;
    const a = archived?.();
    return {
      task: (id) => tasks.get(id) ?? (a?.task.id === id ? a.task : undefined),
      branch: (id) => branches?.get(id) ?? a?.branches.find((b) => b.id === id),
      repoLabel,
      workflow: (id) => m?.workflows.get(id),
      headlessLabel: info.value.headlessLabel,
    };
  });

  const cache = new WeakMap<Session, SessionView>();
  const view = (s: Session): SessionView => reuseView(cache, sessionView(s, lookup.value));
  return { sessions, view };
}
