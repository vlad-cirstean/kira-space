import { validateRefName } from '@kira/git-core';
import { t } from '@theme/i18n';
import { formatRelative } from '@workbench/behaviour/format';
import type { PickerGroup, PickerOption } from '@workbench/lib/pickerOptions';
import type { BaseChoice, BaseExclusion, CandidateBranch, RepoBranches } from './wire';

const SEP = '\u0000';

/** Candidates as branch picker options; `value` is unique across repos. */
export function candidateOptions(
  list: readonly CandidateBranch[],
  repoLabel: (codeRepoId: string) => string,
  now: number,
): PickerOption[] {
  return list.map((b) => {
    const repo = repoLabel(b.codeRepoId);
    const author = b.mine ? t('common.picker.you.label') : b.author;
    return {
      value: `${b.codeRepoId}${SEP}${b.name}`,
      label: b.name,
      detail: [repo, formatRelative(b.lastCommitAt, now), author].filter(Boolean).join(' · '),
      keywords: [repo, b.author],
      dataFont: true,
      testid: b.name,
    };
  });
}

export function candidateOf(value: string): { codeRepoId: string; name: string } {
  const at = value.indexOf(SEP);
  return { codeRepoId: value.slice(0, at), name: value.slice(at + 1) };
}

/** A typed new branch name's error, or null when git accepts it. */
export function newBranchError(name: string): string | null {
  const { valid, error } = validateRefName(name);
  return valid || error === undefined ? null : t(error.key, error.params);
}

function exclusionText(code: BaseExclusion): string | undefined {
  if (code === 'self') return t('space.ade.basePicker.excludedSelf.hint');
  if (code === 'stacked') return t('space.ade.basePicker.excludedStacked.hint');
  return undefined;
}

/** `b:<branchId>` is a planner branch; `r:<ref>` a git ref (`r:` alone = main); `p:<ref>` the previous base. */
export function baseValue(c: BaseChoice | null): string | null {
  if (c === null) return null;
  return c.branchId ? `b:${c.branchId}` : `r:${c.ref}`;
}

export function baseChoiceOf(value: string): BaseChoice {
  const rest = value.slice(2);
  return value.startsWith('b:') ? { ref: '', branchId: rest } : { ref: rest, branchId: '' };
}

export function baseGroups(): PickerGroup[] {
  return [
    { id: 'previous', label: t('space.ade.basePicker.previous.label') },
    { id: 'main', label: t('common.main.label') },
    { id: 'planner', label: t('space.ade.basePicker.planner.label') },
    { id: 'branches', label: t('common.module.branches.label') },
  ];
}

/** The base picker's options: previous base, main, planner branches, then every other branch. */
export function baseOptions(data: RepoBranches): PickerOption[] {
  const out: PickerOption[] = [];
  if (data.previous) {
    out.push({
      value: `p:${data.previous}`,
      label: data.previous,
      group: 'previous',
      dataFont: true,
      testid: 'previous',
    });
  }
  out.push({
    value: 'r:',
    label: data.mainName,
    group: 'main',
    dataFont: true,
    testid: data.mainName,
  });
  for (const b of data.branches) {
    const reason = exclusionText(b.excluded);
    const disabled = b.excluded !== '';
    if (b.branchId !== '') {
      out.push({
        value: `b:${b.branchId}`,
        label: b.name || t('space.ade.board.branchTag.notCreated.label'),
        detail: b.taskTitle,
        group: 'planner',
        dataFont: b.name !== '',
        disabled,
        reason,
        testid: b.name || b.taskTitle,
      });
    } else if (b.name !== data.mainName) {
      out.push({
        value: `r:${b.name}`,
        label: b.name,
        detail: b.local ? undefined : t('common.picker.remote.status'),
        group: 'branches',
        dataFont: true,
        disabled,
        reason,
        testid: b.name,
      });
    }
  }
  return out;
}
