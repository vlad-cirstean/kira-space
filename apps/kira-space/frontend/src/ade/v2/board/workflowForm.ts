import { t } from '@theme/i18n';
import type { StepKind, Workflow, WorkflowStep } from '../wire';
import type { Tone } from './actions';
import { DEFAULT_MAX_RETURNS, defaultResults, reviewDefaultResults } from './stepResults';
import { defaultSandbox } from './stepSandbox';

// Pure helpers behind the Workflows form editor (SPEC2 section 5.1.1). A step id is the slug of its
// name; it follows the name while the step is new (see adeWorkflowDraft) and is frozen once saved.

export const STEP_KINDS: readonly StepKind[] = ['agent', 'script', 'review', 'manual'];

/** Labels for the stored `runsOn` values; the stored text stays as is. */
export function runsOnLabel(value: string): string {
  if (value === 'once') return t('space.ade.workflows.runsOn.once.label');
  if (value === 'each repo') return t('space.ade.workflows.runsOn.each.label');
  if (value.startsWith('only '))
    return t('space.ade.workflows.runsOn.only.label', { name: value.slice(5) });
  return value;
}

export const PLACEHOLDER_TOKENS = '{task} {jira} {repo} {branch} {worktree}';
/** The tokens an outside-repo step has: no repo, no branch. */
export const PLACEHOLDER_TOKENS_OUTSIDE = '{task} {jira} {worktree}';

/** Whether text names a repo or branch, which an outside-repo step does not have. */
export function usesRepoTokens(text: string): boolean {
  return text.includes('{repo}') || text.includes('{branch}');
}

export const KIND_TONE: Record<StepKind, Tone> = {
  agent: 'amber',
  script: 'green',
  review: 'blue',
  manual: 'grey',
};

export function kindLabel(kind: StepKind): string {
  return t(`space.ade.workflows.kind.${kind}.option`);
}

const MAX_STEM = 60;
/** Route words and the finished-task marker; no step takes them as id. */
const RESERVED_IDS: ReadonlySet<string> = new Set(['next', 'end', 'stop', 'done']);

/** Lowercase `text` to the id alphabet (the Go side's `slug`); `step` when nothing is left. */
function slugOf(text: string): string {
  const base = text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  const cut = base.length > MAX_STEM ? base.slice(0, MAX_STEM).replace(/-+$/g, '') : base;
  return cut || 'step';
}

/** The step id for `name`: its slug, suffixed `-2`, `-3` ... (within 60 characters) while taken. Run
 *  keys stay safe because a started task reads its own snapshot of the workflow. */
export function stepIdFor(name: string, taken: readonly string[]): string {
  const used = new Set(taken);
  const base = slugOf(name);
  for (let n = 1; ; n++) {
    const suffix = n < 2 ? '' : `-${n}`;
    const stem =
      base.length + suffix.length > MAX_STEM
        ? base.slice(0, MAX_STEM - suffix.length).replace(/-+$/g, '')
        : base;
    const id = stem + suffix;
    if (!used.has(id) && !RESERVED_IDS.has(id)) return id;
  }
}

/** The fields a kind owns at their defaults; the other kinds' fields hold their zero value. */
function kindDefaults(kind: StepKind): Omit<WorkflowStep, 'id' | 'name'> {
  const base: Omit<WorkflowStep, 'id' | 'name'> = {
    kind,
    status: '',
    skip: false,
    session: false,
    command: '',
    runsOn: '',
    outsideRepo: false,
    before: '',
    results: [],
    maxReturns: 0,
    timeout: '',
    prompt: '',
    allowedTools: [],
    dbMcp: kind === 'agent' || kind === 'review',
    smartScript: '',
    params: {},
    review: null,
    sandbox: defaultSandbox(),
    agent: '',
    settings: {},
  };
  if (kind === 'manual') return base;
  const automatic = { ...base, runsOn: 'each repo' as const, before: 'auto' as const };
  if (kind === 'review') {
    return {
      ...automatic,
      results: reviewDefaultResults(),
      review: { sendTo: 'implementer', implementer: '' },
    };
  }
  return {
    ...automatic,
    results: defaultResults(),
    maxReturns: DEFAULT_MAX_RETURNS,
    timeout: kind === 'script' ? '10m' : '1h',
  };
}

export function newStep(kind: StepKind, taken: readonly string[]): WorkflowStep {
  const name = kindLabel(kind);
  return { id: stepIdFor(name, taken), name, ...kindDefaults(kind) };
}

/** The step turned into `kind`: id, name, status, skip and `runs_on` (when the kind has one) stay,
 *  everything else goes back to the kind's defaults, except the stage settings an agent or review keeps. */
export function withKind(step: WorkflowStep, kind: StepKind): WorkflowStep {
  if (step.kind === kind) return step;
  const fresh = kindDefaults(kind);
  const keepOutside = step.outsideRepo && (kind === 'agent' || kind === 'script');
  return {
    ...fresh,
    id: step.id,
    name: step.name,
    skip: step.skip,
    status: step.status,
    outsideRepo: keepOutside,
    runsOn: fresh.runsOn === '' || keepOutside ? '' : step.runsOn || fresh.runsOn,
    settings: kind === 'agent' || kind === 'review' ? step.settings : {},
  };
}

/** Whether changing the step's kind would drop something typed: a prompt, command or session prompt. */
export function hasContent(step: WorkflowStep): string {
  if (step.prompt) return step.kind === 'manual' ? 'sessionPrompt' : 'prompt';
  if (step.command) return 'command';
  return step.smartScript ? 'smartScript' : '';
}

export function runnableSteps(steps: readonly WorkflowStep[]): number {
  return steps.filter((s) => !s.skip).length;
}

export function parseTools(text: string): string[] {
  return text
    .split(/[,\n]/)
    .map((t) => t.trim())
    .filter(Boolean);
}

export function formatTools(tools: readonly string[]): string {
  return tools.join(', ');
}

export function cloneWorkflow(wf: Workflow): Workflow {
  return JSON.parse(JSON.stringify(wf)) as Workflow;
}
