import type { Workflow, WorkflowStep } from '../wire';

// Pure helpers behind library steps: a workflow step with `use` set references a library step and
// holds its effective values; the fields that differ from the library step are its overrides.

/** Step fields a reference can override, in the order the inspector lists them. */
const OVERRIDABLE = [
  'name',
  'status',
  'agent',
  'smartScript',
  'params',
  'prompt',
  'command',
  'session',
  'runsOn',
  'outsideRepo',
  'before',
  'timeout',
  'allowedTools',
  'settings',
  'sandbox',
  'review',
  'maxReturns',
  'results',
] as const satisfies readonly (keyof WorkflowStep)[];
export type Overridable = (typeof OVERRIDABLE)[number];

function stable(v: unknown): string {
  return JSON.stringify(v, (_k, x: unknown) =>
    x && typeof x === 'object' && !Array.isArray(x)
      ? Object.fromEntries(Object.entries(x).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)))
      : x,
  );
}

/** The fields where `step` differs from the library step it references. */
export function overriddenKeys(step: WorkflowStep, lib: WorkflowStep): Overridable[] {
  return OVERRIDABLE.filter((k) => stable(step[k]) !== stable(lib[k]));
}

/** `step` with one field back at the library step's value. */
export function resetKey(step: WorkflowStep, lib: WorkflowStep, key: Overridable): WorkflowStep {
  return { ...step, [key]: JSON.parse(JSON.stringify(lib[key])) as unknown };
}

/** A reference to library step `lib`: its values, no overrides. The id is the library id when the
 *  workflow does not hold it yet, else `fresh`. */
export function referenceStep(
  lib: WorkflowStep,
  taken: readonly string[],
  fresh: string,
): WorkflowStep {
  const copy = JSON.parse(JSON.stringify(lib)) as WorkflowStep;
  return {
    ...copy,
    id: taken.includes(lib.id) ? fresh : lib.id,
    skip: false,
    use: lib.id,
    promptExtra: '',
  };
}

/** The inline step a reference becomes when detached: the extra prompt folds into the prompt. */
export function detachedStep(step: WorkflowStep): WorkflowStep {
  const { use: _use, promptExtra, ...rest } = step;
  const extra = promptExtra?.trim() ?? '';
  const prompt = extra && step.smartScript === '' ? `${step.prompt}\n\n${extra}` : step.prompt;
  return { ...rest, prompt };
}

/** A one-step workflow around a library step, so the step inspector edits it through the workflow draft. */
export function libraryWorkflow(step: WorkflowStep): Workflow {
  return { id: step.id, name: step.name, steps: [step] };
}
