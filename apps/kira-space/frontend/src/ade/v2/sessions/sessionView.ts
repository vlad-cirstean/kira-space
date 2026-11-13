import { t } from '@theme/i18n';
import { compareNatural } from '@workbench/behaviour/match';
import { type ActivityKind, activityKind, agentSessionName, shortId } from '../activity';
import type { Branch, Run, Session, Task, Workflow } from '../wire';

// A session as the Sessions tab and All sessions list show it (mockup `termFor` / `agents`). Pure:
// the lookups come in as data, so a session of an archived task, which the board no longer lists,
// degrades to its ids instead of failing.

/** The branch fields a session label reads; an archived branch has no worktree. */
type LookupBranch = Pick<Branch, 'name' | 'codeRepoId'> & { worktree?: string };

export interface SessionLookup {
  task: (id: string) => Task | undefined;
  branch: (id: string) => LookupBranch | undefined;
  repoLabel: (codeRepoId: string) => string;
  workflow: (id: string) => Workflow | undefined;
  /** The background run's command line as the provider shows it (`claude -p`). */
  headlessLabel: string;
}

export interface SessionView {
  session: Session;
  kind: ActivityKind;
  headless: boolean;
  /** Repo nickname; `''` for a task-level (spec) session. */
  repo: string;
  /** Branch name; `''` for a task-level session. */
  branch: string;
  worktree: string;
  step: string;
  run: Run | null;
  /** Strip tab: `Implement · repo` / `Implement · Create PR · repo`; `Review agent` for the review agent. */
  tabName: string;
  badge: string;
  /** Last characters of the session id, as the rows show it. */
  shortId: string;
  /** All sessions kind column: `<headless label> · <step>` / `TUI · interactive` / `TUI · spec session`. */
  allLabel: string;
}

function stepNameOf(workflow: Workflow | undefined, stepId: string): string {
  if (!stepId) return '';
  return workflow?.steps.find((x) => x.id === stepId)?.name ?? stepId;
}

export function sessionView(s: Session, look: SessionLookup): SessionView {
  const task = look.task(s.taskId);
  const branch = s.branchId ? look.branch(s.branchId) : undefined;
  const headless = s.mode === 'headless';
  const repo = branch ? look.repoLabel(branch.codeRepoId) : '';
  const branchName = branch?.name ?? '';
  const workflow = task?.workflow ?? look.workflow(task?.workflowId ?? '');
  const stepName = stepNameOf(workflow, s.stepId);
  const step = headless ? stepName : '';
  const run = task?.runs.find((r) => r.id === s.runId) ?? null;
  const specLabel = t('space.ade.needs.scope.spec.label');
  const id = shortId(s.id);
  const review = s.purpose === 'review';
  const named = (...parts: string[]): string => parts.filter(Boolean).join(' · ');
  const reviewAgent = t('space.ade.review.agent.label');
  const resumed = (name: string): string =>
    s.resumes ? t('space.ade.sessionList.resumed.label', { name }) : name;
  const tabName = review
    ? reviewAgent
    : headless
      ? named(step, repo)
      : resumed(named(stepName || agentSessionName(id), repo || specLabel));
  return {
    session: s,
    kind: activityKind(s),
    headless,
    repo,
    branch: branchName,
    worktree: branch?.worktree || s.cwd,
    step,
    run,
    tabName,
    badge: headless ? look.headlessLabel : 'TUI',
    shortId: id,
    allLabel: review
      ? `TUI · ${reviewAgent}`
      : headless
        ? named(look.headlessLabel, step)
        : s.branchId
          ? named('TUI', stepName, t('space.ade.sessionList.interactive.label'))
          : named('TUI', stepName, t('space.ade.sessionList.specSession.label')),
  };
}

/** Running sessions: interactive first, then the most recently active. */
export function byRunningOrder(a: Session, b: Session): number {
  const mode = Number(a.mode === 'headless') - Number(b.mode === 'headless');
  return mode || b.lastActiveAt - a.lastActiveAt || compareNatural(a.id, b.id);
}

export function byRecent(a: Session, b: Session): number {
  return b.lastActiveAt - a.lastActiveAt || compareNatural(a.id, b.id);
}

/** `fresh`, or the view cached for the same session object when every derived field is unchanged,
 *  so list rows keep their props across board pushes that touch other tasks. */
export function reuseView(cache: WeakMap<Session, SessionView>, fresh: SessionView): SessionView {
  const prev = cache.get(fresh.session);
  if (
    prev &&
    (Object.keys(fresh) as (keyof SessionView)[]).every((k) => Object.is(prev[k], fresh[k]))
  )
    return prev;
  cache.set(fresh.session, fresh);
  return fresh;
}
