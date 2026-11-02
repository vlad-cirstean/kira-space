import { t } from '@theme/i18n';
import { plainText } from '@theme/varText';
import {
  type ActivityKind,
  activityKind,
  activityLabel,
  agentSessionName,
  shortId,
} from '../activity';
import type { BranchGraph } from '../board/branchGraph';
import { type RebaseAct, stackIds } from '../board/rebaseActions';
import type {
  ArchiveRisk,
  BaseChoice,
  Branch,
  BranchRisk,
  RebaseBlocker,
  RebasePreview,
  Session,
  Task,
} from '../wire';

// Pure templates and view model of the Claude dialog (mockup `dlg`).
// No Vue, no clock; `AdeAgentDialog.vue` wraps `composeDialog` in a computed. The session and start
// templates mirror the server defaults (`composeManualMessage`, `composeStartMessage`): an unedited
// message is sent as `''` and the server composes it, so these only show what it will say.

type DialogKind = 'rebase' | 'queue' | 'changeBase' | 'merge' | 'session' | 'start' | 'archive';

export interface DialogTarget {
  branchId: string;
  /** A session record id, or `'new'` for a fresh launch. */
  choice: string;
}

export interface DialogSpec {
  kind: DialogKind;
  title: string;
  /** rebase / queue / changeBase: the branch being rebased; merge / start: the branch. */
  branchId?: string;
  /** rebase / changeBase: the picked base, `null` = the current one. */
  onto?: BaseChoice | null;
  /** queue: the review branch to rebase onto and queue after. */
  queueWith?: string;
  /** changeBase: the branch is not created yet, so only the stored base changes. */
  draft?: boolean;
  /** rebase / queue / changeBase: the stack the busy check covers. */
  stack?: string[];
  /** merge: the integration branch. */
  target?: string;
  /** session / archive: the task. */
  taskId?: string;
  /** archive: only the branches at risk. */
  risk?: ArchiveRisk;
  targets: DialogTarget[];
}

export interface DialogState {
  /** `null` = the unedited template. */
  msg: string | null;
  push: boolean;
  override: boolean;
  autostash: boolean;
  /** The rebase kinds' server-composed prompt; `null` while it loads or for a draft. */
  preview: RebasePreview | null;
}

export interface DialogCtx {
  graph: BranchGraph;
  /** Sessions with TUI activity merged in. */
  sessions: readonly Session[];
  worktreeBasePath: string;
  repoState: (codeRepoId: string) => { mainName: string; remote: string };
  repo: (codeRepoId: string) => { nick: string; name: string; root: string };
  taskTitle: (taskId: string) => string;
  /** The task's current step when it is automatic and not done, `''` otherwise. */
  openStep: (taskId: string) => string;
  /** The task's current step: its name and the first-message prompt of a manual step; `null` without one. */
  currentStep: (taskId: string) => { name: string; prompt: string } | null;
  /** What the last stage move left for the task's current step; `null` when it went elsewhere. */
  handoverInto: (taskId: string) => { from: string; body: string } | null;
  /** The Kira Space note (`headless.space.suffix`, rendered in Go) that precedes the tool line. */
  spaceNote: string;
  /** The tool line (`ade.handoverLine`, rendered in Go) that ends a launch message. */
  handoverLine: string;
}

interface DialogChip {
  value: string;
  label: string;
  on: boolean;
}

export interface DialogView {
  title: string;
  message: string;
  edited: boolean;
  /** A blank message disables Send. */
  sendDisabled: boolean;
  blocked: boolean;
  busy: { text: string; kind: ActivityKind }[];
  busyShown: boolean;
  busyTitle: string;
  overrideLabel: string;
  overridden: boolean;
  /** A running background run on the stack: blocks with no override. */
  headless: string;
  /** The message box shows: every kind but a draft's Change base. */
  showMessage: boolean;
  /** The rebase prompt's read-only closing instructions. */
  suffix: string;
  blockers: RebaseBlocker[];
  /** A dirty blocker the Autostash switch can clear. */
  canAutostash: boolean;
  autostashOn: boolean;
  /** The branches already sit on the base: Send only stores it. */
  noOp: boolean;
  canPush: boolean;
  pushOn: boolean;
  pushLabel: string;
  isArchive: boolean;
  riskText: string;
  targets: {
    repo: string;
    repoId: string;
    branch: string;
    branchId: string;
    options: DialogChip[];
  }[];
  sendLabel: string;
}

// ---- lookups

function branchOf(ctx: DialogCtx, id: string): Branch | undefined {
  return ctx.graph.byBranch.get(id);
}

function taskOf(ctx: DialogCtx, id: string): Task | undefined {
  return ctx.graph.byTask.get(id);
}

function lastSegment(name: string): string {
  return name.split('/').at(-1) || name;
}

/** The server's worktree directory segment: `/` in a repo name becomes `-`. */
function repoSegment(name: string): string {
  return name.replaceAll('/', '-');
}

function baseDir(ctx: DialogCtx): string {
  return ctx.worktreeBasePath.replace(/\/+$/, '');
}

/** The branch's worktree, else where the server would create it. */
function wtOf(ctx: DialogCtx, b: Branch): string {
  if (b.worktree) return b.worktree;
  const repo = ctx.repo(b.codeRepoId);
  return `${baseDir(ctx)}/${repoSegment(repo.name)}/${lastSegment(b.name)}`;
}

function runningTui(ctx: DialogCtx, branchId: string): Session[] {
  return ctx.sessions.filter(
    (s) => s.branchId === branchId && s.mode === 'tui' && s.state === 'running',
  );
}

function targetsFor(ctx: DialogCtx, ids: readonly string[]): DialogTarget[] {
  return ids.map((branchId) => ({
    branchId,
    choice: runningTui(ctx, branchId)[0]?.id ?? 'new',
  }));
}

// ---- openers

/** The dialog behind a `rebase` or `queue` act: the server composes its prompt. */
export function rebaseSpec(
  ctx: DialogCtx,
  act: Extract<RebaseAct, { kind: 'rebase' | 'queue' }>,
): DialogSpec {
  return {
    kind: act.kind,
    title: plainText(act.label),
    branchId: act.branchId,
    onto: act.kind === 'rebase' ? act.onto : null,
    queueWith: act.kind === 'queue' ? act.withId : '',
    stack: stackIds(ctx.graph, act.branchId),
    targets: [],
  };
}

export function changeBaseSpec(
  ctx: DialogCtx,
  act: Extract<RebaseAct, { kind: 'changeBase' }>,
): DialogSpec {
  return {
    kind: 'changeBase',
    title: t('space.ade.agentDialog.changeBase.title'),
    branchId: act.branchId,
    onto: null,
    draft: act.draft,
    stack: stackIds(ctx.graph, act.branchId),
    targets: [],
  };
}

export function mergeSpec(ctx: DialogCtx, branchId: string, target: string): DialogSpec {
  const stale =
    branchOf(ctx, branchId)?.integration.find((i) => i.target === target)?.status === 'stale';
  return {
    kind: 'merge',
    title: t(
      stale ? 'space.ade.agentDialog.reMerge.title' : 'space.ade.board.fixMenu.merge.label',
      {
        target,
      },
    ),
    branchId,
    target,
    targets: targetsFor(ctx, [branchId]),
  };
}

export function sessionSpec(ctx: DialogCtx, taskId: string): DialogSpec {
  const step = ctx.currentStep(taskId)?.name ?? t('space.common.step.label');
  return {
    kind: 'session',
    title: t('space.ade.agentDialog.step.title', { step }),
    taskId,
    targets: [],
  };
}

export function startSpec(branchId: string): DialogSpec {
  return { kind: 'start', title: t('space.ade.agentDialog.start.title'), branchId, targets: [] };
}

/** Work that archiving would lose: uncommitted files or commits no integration branch holds. */
export const atRisk = (r: BranchRisk): boolean => r.dirty.length > 0 || r.unmerged > 0;

export function archiveSpec(ctx: DialogCtx, risk: ArchiveRisk): DialogSpec {
  const atRiskBranches = risk.branches.filter(atRisk);
  return {
    kind: 'archive',
    title: t('space.ade.agentDialog.archive.title'),
    taskId: risk.taskId,
    risk: { ...risk, branches: atRiskBranches },
    targets: targetsFor(
      ctx,
      atRiskBranches.map((r) => r.branchId),
    ),
  };
}

// ---- templates

const ASK = 'If anything is unclear, ask me before changing anything.';

/** Worktree of the integration branch `T`, kept apart from the branch's own. */
function mergeWorktree(ctx: DialogCtx, b: Branch, target: string): string {
  return `${baseDir(ctx)}/${repoSegment(ctx.repo(b.codeRepoId).name)}/_${target}`;
}

function mergeMessage(ctx: DialogCtx, spec: DialogSpec, push: boolean): string {
  const b = branchOf(ctx, spec.branchId ?? '') as Branch;
  const target = spec.target ?? '';
  const remote = ctx.repoState(b.codeRepoId).remote || 'origin';
  const twt = mergeWorktree(ctx, b, target);
  return [
    `Merge ${b.name} into ${target} (repo ${ctx.repo(b.codeRepoId).nick}).`,
    `1. git fetch ${remote}`,
    `2. Use the ${target} worktree at ${twt} (create it if missing: git worktree add ${twt} ${remote}/${target} -B ${target})`,
    `3. In ${twt}: git merge ${b.name}`,
    `Resolve any conflicts. ${push ? `Then push: git push ${remote} ${target}` : 'Do not push.'}`,
    ASK,
  ].join('\n');
}

function archiveMessage(ctx: DialogCtx, spec: DialogSpec): string {
  const lines = [
    `Task "${ctx.taskTitle(spec.taskId ?? '')}" is being archived and its worktrees will be deleted.`,
  ];
  for (const r of spec.risk?.branches ?? []) {
    const b = branchOf(ctx, r.branchId);
    if (!b) continue;
    lines.push(
      `- ${ctx.repo(b.codeRepoId).nick} · ${b.name} · ${r.worktree || wtOf(ctx, b)}` +
        (r.dirty.length ? ` · uncommitted: ${r.dirty.map((d) => d.path).join(', ')}` : '') +
        (r.unmerged ? ` · commits not merged into main: ${r.unmerged}` : ''),
    );
  }
  lines.push('Before they are deleted: ');
  return lines.join('\n');
}

/** One pass, so a value holding a placeholder is not expanded again. */
function substitute(text: string, vars: Record<string, string>): string {
  return text.replace(/\{(task|jira|repo|branch|worktree)\}/g, (_, k: string) => vars[k] ?? '');
}

function jiraLine(t: Task | undefined): string {
  return t?.jira ? `- Jira: ${t.jira.key} ${t.jira.url}` : '';
}

function repoLine(ctx: DialogCtx, b: Branch): string {
  const repo = ctx.repo(b.codeRepoId);
  return b.kind === 'mine' && b.name !== ''
    ? `- Repo: ${repo.nick} · Branch: ${b.name} · Worktree: ${wtOf(ctx, b)}`
    : `- Repo: ${repo.nick} (read only, in ${repo.root})`;
}

/** The handover block, Kira Space note and tool line that end a launch message, as `ade.handoverBlock`, `headless.space.suffix` and `ade.handoverLine` render. */
function handoverLines(ctx: DialogCtx, taskId: string): string[] {
  const h = ctx.handoverInto(taskId);
  return [
    ...(h ? [`Handover from ${h.from}:\n${h.body}`] : []),
    ...(ctx.spaceNote ? [ctx.spaceNote] : []),
    ...(ctx.handoverLine ? [ctx.handoverLine] : []),
  ];
}

function sessionMessage(ctx: DialogCtx, spec: DialogSpec): string {
  const taskId = spec.taskId ?? '';
  const t = taskOf(ctx, taskId);
  const title = ctx.taskTitle(taskId);
  const step = ctx.currentStep(taskId);
  const branches = (t?.branchIds ?? []).flatMap((id) => branchOf(ctx, id) ?? []);
  const lines = [`${step?.name ?? 'Step'}: ${title}`];
  const jira = jiraLine(t);
  if (jira) lines.push(jira);
  const own = branches.filter((b) => b.kind === 'mine' && b.name !== '');
  for (const b of branches) lines.push(repoLine(ctx, b));
  // The server adds the task folder's path to this line.
  if (!branches.length)
    lines.push('- No repo yet. Pick repos and branches with the Kira Space tools.');
  if (t?.notes) lines.push(`- Notes: ${t.notes}`);
  if (step?.prompt) {
    lines.push(
      substitute(step.prompt, {
        task: title,
        jira: t?.jira?.key ?? '',
        repo: own.map((b) => ctx.repo(b.codeRepoId).nick).join(', '),
        branch: own.map((b) => b.name).join(', '),
        worktree: own.map((b) => wtOf(ctx, b)).join(', '),
      }),
    );
  }
  lines.push(...handoverLines(ctx, taskId));
  return lines.join('\n');
}

function startMessage(ctx: DialogCtx, spec: DialogSpec): string {
  const b = branchOf(ctx, spec.branchId ?? '') as Branch;
  const t = taskOf(ctx, b.taskId);
  const lines = [`Task: ${ctx.taskTitle(b.taskId)}`];
  const jira = jiraLine(t);
  if (jira) lines.push(jira);
  lines.push(repoLine(ctx, b));
  const step = ctx.openStep(b.taskId);
  if (step) lines.push(`Step: ${step}`);
  lines.push(...handoverLines(ctx, b.taskId));
  return lines.join('\n');
}

export function templateFor(ctx: DialogCtx, spec: DialogSpec, state: DialogState): string {
  switch (spec.kind) {
    case 'rebase':
    case 'queue':
    case 'changeBase':
      return state.preview?.prompt ?? '';
    case 'merge':
      return mergeMessage(ctx, spec, state.push);
    case 'archive':
      return archiveMessage(ctx, spec);
    case 'session':
      return sessionMessage(ctx, spec);
    default:
      return startMessage(ctx, spec);
  }
}

// ---- view

/** The delivery target actually used: a stored session choice that is no longer running falls back
 *  to the first running one, else a new session. The display and the delivery share this. */
export function resolvedChoice(ctx: DialogCtx, tg: DialogTarget): string {
  const running = runningTui(ctx, tg.branchId);
  if (!running.length) return 'new';
  return running.some((s) => s.id === tg.choice) ? tg.choice : (running[0] as Session).id;
}

/** The kinds that rewrite a branch through a background rebase run. */
export function isRebaseKind(spec: DialogSpec): boolean {
  return spec.kind === 'rebase' || spec.kind === 'queue' || spec.kind === 'changeBase';
}

function busyFor(ctx: DialogCtx, spec: DialogSpec): DialogView['busy'] {
  if (!isRebaseKind(spec) || spec.draft) return [];
  const out: DialogView['busy'] = [];
  for (const id of spec.stack ?? []) {
    const b = branchOf(ctx, id);
    if (!b) continue;
    for (const s of runningTui(ctx, id)) {
      const kind = activityKind(s);
      if (kind === 'working' || kind === 'waiting') {
        out.push({
          text: `${ctx.repo(b.codeRepoId).nick} · ${b.name} · ${agentSessionName(shortId(s.id))} · ${activityLabel(kind)}`,
          kind,
        });
      }
    }
  }
  return out;
}

function headlessFor(ctx: DialogCtx, spec: DialogSpec): string {
  if (!isRebaseKind(spec) || spec.draft) return '';
  for (const id of spec.stack ?? []) {
    if (
      ctx.sessions.some((s) => s.branchId === id && s.mode === 'headless' && s.state === 'running')
    ) {
      return t('space.ade.agentDialog.headless.body', { branch: branchOf(ctx, id)?.name ?? id });
    }
  }
  return '';
}

function riskText(ctx: DialogCtx, spec: DialogSpec): string {
  if (spec.kind !== 'archive') return '';
  return (spec.risk?.branches ?? [])
    .map((r) => {
      const b = branchOf(ctx, r.branchId);
      const facts = [
        r.dirty.length
          ? t('space.ade.agentDialog.riskUncommitted.label', { count: r.dirty.length })
          : '',
        r.unmerged ? t('space.ade.agentDialog.riskUnmerged.label', { count: r.unmerged }) : '',
      ].filter(Boolean);
      return t('space.ade.agentDialog.riskRepo.label', {
        repo: b ? ctx.repo(b.codeRepoId).nick : r.branchId,
        facts: facts.join(', '),
      });
    })
    .join(' · ');
}

function sendLabel(spec: DialogSpec, overridden: boolean, noOp: boolean): string {
  if (overridden) return t('space.ade.agentDialog.sendAnyway.label');
  if (spec.kind === 'changeBase' && (spec.draft || noOp))
    return t('space.ade.agentDialog.saveBase.label');
  switch (spec.kind) {
    case 'rebase':
    case 'queue':
    case 'changeBase':
      return t('space.ade.agentDialog.runInBackground.label');
    case 'session':
      return t('space.ade.board.taskMenu.openSession.label');
    case 'start':
      return t('common.action.start.label');
    case 'archive':
      return t('space.ade.agentDialog.sendAndArchive.label');
    default:
      return t('space.ade.agentDialog.send.label');
  }
}

export function composeDialog(ctx: DialogCtx, spec: DialogSpec, state: DialogState): DialogView {
  const template = templateFor(ctx, spec, state);
  const message = state.msg ?? template;
  const busy = busyFor(ctx, spec);
  const headless = headlessFor(ctx, spec);
  const overridden = busy.length > 0 && state.override;
  const rebase = isRebaseKind(spec);
  const draft = spec.kind === 'changeBase' && spec.draft === true;
  const blockers = rebase && !draft ? (state.preview?.blockers ?? []) : [];
  const dirty = blockers.some((b) => b.kind === 'dirty');
  const noOp = rebase && !draft && state.preview?.noOp === true;
  const sendDisabled = !rebase
    ? message.trim().length === 0
    : draft
      ? spec.onto == null
      : state.preview === null ||
        blockers.some((b) => b.kind !== 'dirty') ||
        (dirty && !state.autostash) ||
        (!noOp && message.trim().length === 0);
  const targets: DialogView['targets'] = spec.targets.map((tg) => {
    const b = branchOf(ctx, tg.branchId);
    const running = runningTui(ctx, tg.branchId);
    const chosen = resolvedChoice(ctx, tg);
    const opts = running.length
      ? running.map((s) => ({
          value: s.id,
          label: agentSessionName(shortId(s.id)),
        }))
      : [{ value: 'new', label: t('common.newSession.label') }];
    return {
      repo: b ? ctx.repo(b.codeRepoId).nick : '',
      repoId: b?.codeRepoId ?? '',
      branch: b?.name ?? tg.branchId,
      branchId: tg.branchId,
      options: opts.map((o) => ({
        value: o.value,
        label:
          running.length === 1
            ? t('space.ade.agentDialog.onlyAgent.label', { session: o.label })
            : o.label,
        on: chosen === o.value,
      })),
    };
  });
  return {
    title: spec.title,
    message,
    edited: state.msg !== null,
    sendDisabled,
    blocked: headless !== '' || (busy.length > 0 && !state.override),
    busy,
    busyShown: busy.length > 0,
    busyTitle: t(
      state.override
        ? 'space.ade.agentDialog.busyOverridden.hint'
        : 'space.ade.agentDialog.busy.hint',
    ),
    overrideLabel: t(
      state.override
        ? 'space.ade.agentDialog.undoOverride.label'
        : 'space.ade.agentDialog.override.label',
    ),
    overridden,
    headless,
    showMessage: !draft && !noOp,
    suffix: rebase && !draft ? (state.preview?.suffix ?? '') : '',
    blockers,
    canAutostash: dirty,
    autostashOn: state.autostash,
    noOp,
    canPush: (rebase && !draft && !noOp) || spec.kind === 'merge',
    pushOn: state.push,
    pushLabel:
      spec.kind === 'merge'
        ? t('space.ade.agentDialog.pushMerge.label', { target: spec.target ?? '' })
        : t('space.ade.agentDialog.pushRebase.label'),
    isArchive: spec.kind === 'archive',
    riskText: riskText(ctx, spec),
    targets,
    sendLabel: sendLabel(spec, overridden, noOp),
  };
}
