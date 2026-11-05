import { t } from '@theme/i18n';
import type { StepKind, Workflow, WorkflowStep } from '../wire';
import type { Tone } from './actions';
import { DEFAULT_MAX_RETURNS, defaultResults, reviewDefaultResults } from './stepResults';
import { defaultSandbox } from './stepSandbox';

// Pure helpers behind the Workflows form editor (SPEC2 section 5.1.1). Step ids are run keys: they
// are minted once, here, and never follow a rename.

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

const ID_SUFFIX_LENGTH = 8;

/** `step-<8 hex>`, never in `taken`. Runs are keyed by these ids, so a removed id must not come
 *  back: the lowest free `step-N` would hand a new step the old one's runs. */
export function newId(taken: readonly string[]): string {
  const used = new Set(taken);
  let id: string;
  do id = `step-${crypto.randomUUID().slice(0, ID_SUFFIX_LENGTH)}`;
  while (used.has(id));
  return id;
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
    smartScript: '',
    params: {},
    review: null,
    sandbox: defaultSandbox(kind),
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
  return {
    id: newId(taken),
    name: t('space.ade.workflows.step.defaultName.label'),
    ...kindDefaults(kind),
  };
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
