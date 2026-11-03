import type { TextPart } from '@theme/varText';

type ResultLike = { id: string; ok: boolean; description: string };

/** True for no results or the implicit done/failed pair; mirrors Go `agentmcp.IsImplicit`. */
function isImplicit(results: readonly ResultLike[]): boolean {
  return (
    results.length === 0 ||
    (results.length === 2 &&
      results[0]?.id === 'done' &&
      !!results[0].ok &&
      !results[0].description &&
      results[1]?.id === 'failed' &&
      !results[1].ok &&
      !results[1].description)
  );
}

/** Vars of `headless.finishStep.suffix` for a step's results; the text itself renders in Go. Keys
 *  stay in alphabetical order, the order the Go flow tests write the UI fixtures in. */
export function finishVars(results: readonly ResultLike[]): Record<string, unknown> {
  return {
    implicit: isImplicit(results),
    results: results.map((r) => ({ description: r.description, id: r.id, ok: r.ok })),
  };
}

/** A preview as one string: literal text and variable values. */
export function previewPlain(
  parts: readonly { text: string; var: string; value: string }[],
): string {
  return parts.map((p) => (p.var === '' ? p.text : p.value)).join('');
}

/** The suffix preview as parts: each variable is a chip, literal text stays plain. */
export function previewTextParts(
  parts: readonly { text: string; var: string; value: string }[],
): TextPart[] {
  return parts.map((p) => (p.var === '' ? p.text : { name: p.var, value: p.value }));
}

const MAX_BRANCH_SLUG = 40;

/** Branch-name slug of a task title; mirrors Go `slug` (internal/ade/setup.go). */
export function branchSlug(title: string): string {
  const out = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, MAX_BRANCH_SLUG)
    .replace(/-+$/g, '');
  return out === '' ? 'task' : out;
}

export interface RunMessageInput {
  /** Display title, the value of `{task}`. */
  title: string;
  jira: { key: string; url: string } | null;
  /** The agent step being run. */
  step: { name: string; prompt: string; results: readonly ResultLike[] };
  /** Its 1-based position among the workflow's automatic steps, and their count. */
  stepNumber: number;
  stepCount: number;
  /** The task has no mine branch: the step runs in the task folder. */
  noRepo?: boolean;
}

/** Vars of `ade.step.prompt` for the Run dialog, keys in alphabetical order. Per-run placeholders (`{repo}` `{branch}`
 *  `{worktree}`) stay literal for the server to fill per run. */
export function runMessageVars(i: RunMessageInput): Record<string, unknown> {
  const first = i.step;
  const jira = i.jira?.key ?? '';
  const prompt = first.prompt
    .replaceAll('{task}', i.title)
    .replaceAll('{jira}', jira)
    .replace(/\n+$/, '');
  return {
    branch: '{branch}',
    context: [],
    extra: '',
    finish: finishVars(first.results),
    jiraKey: jira,
    jiraURL: i.jira?.url ?? '',
    message: '',
    repo: i.noRepo ? '' : '{repo}',
    stepCount: i.stepCount,
    stepName: first.name,
    stepNumber: i.stepNumber,
    stepPrompt: prompt,
    title: i.title,
    worktree: '{worktree}',
  };
}

/** The editable part of a rendered `ade.step.prompt`: it ends with a blank line, the finish
 *  instruction and a newline, which the dialog shows read-only. */
export function editablePart(rendered: string, suffix: string): string {
  const tail = `\n\n${suffix}\n`;
  return (rendered.endsWith(tail) ? rendered.slice(0, -tail.length) : rendered).replace(/\n+$/, '');
}
