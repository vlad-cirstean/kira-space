import {
  BUILTIN_VARS,
  type ScriptDirMode,
  type ScriptKind,
  type ScriptParam,
  type ScriptSchedule,
} from '@shared/domain/scripts';
import { t } from '@theme/i18n';
import { varsUsed } from './smart/smartSettings';

export type EditorTab = 'script' | 'params' | 'schedule';

export interface EditorDraft {
  kind: ScriptKind;
  name: string;
  body: string;
  dirMode: ScriptDirMode;
  workingDir: string;
  maxBudgetUsd: number;
  noCostCap: boolean;
  params: readonly ScriptParam[];
  schedule: ScriptSchedule | null;
  scheduleOk: boolean;
}

export interface EditorErrors {
  name?: string;
  body?: string;
  folder?: string;
  budget?: string;
  /** Messages per parameter row, same index as the draft's params. */
  params: string[][];
  cron?: string;
  /** The schedule's cron or zone is refused; ScheduleFields shows why. */
  scheduleInvalid: boolean;
}

/** Which empty-field errors to report: a field nobody touched yet is not an error to show. */
export interface Shown {
  name: boolean;
  body: boolean;
  folder: boolean;
}

export const ALL_SHOWN: Shown = { name: true, body: true, folder: true };

const NAME_RE = /^[a-z][a-z0-9_]{0,31}$/;

function paramErrors(d: EditorDraft): string[][] {
  const used = d.kind === 'smart' ? varsUsed(d.body) : [];
  return d.params.map((p, i) => {
    const out: string[] = [];
    const name = p.name.trim();
    if (!NAME_RE.test(name)) out.push(t('automations.error.paramName.error'));
    else if ((BUILTIN_VARS as readonly string[]).includes(name))
      out.push(t('automations.error.paramReserved.error', { name }));
    else if (d.params.findIndex((o) => o.name.trim() === name) !== i)
      out.push(t('automations.error.paramTwice.error', { name }));
    if (p.type !== 'text' && p.options.length === 0)
      out.push(t('automations.error.paramOption.error'));
    if (p.secret && used.includes(name)) out.push(t('automations.script.secretInPrompt.error'));
    return out;
  });
}

/** Client-side mirror of the Go checks, to flag a tab before Save. Go stays the authority. */
export function editorErrors(d: EditorDraft, shown: Shown): EditorErrors {
  const out: EditorErrors = { params: paramErrors(d), scheduleInvalid: false };
  if (shown.name && d.name.trim() === '') out.name = t('automations.error.nameRequired.error');
  if (shown.body && d.body.trim() === '')
    out.body = t(
      d.kind === 'smart'
        ? 'automations.error.promptRequired.error'
        : 'automations.error.commandRequired.error',
    );
  if (shown.folder && d.dirMode === 'fixed' && d.workingDir.trim() === '')
    out.folder = t('automations.error.folderRequired.error');
  if (d.kind === 'smart' && !d.noCostCap && !(d.maxBudgetUsd > 0))
    out.budget = t('automations.error.budget.error');
  if (d.schedule) {
    if (d.schedule.cron.trim() === '') out.cron = t('automations.error.repeatRequired.error');
    else out.scheduleInvalid = !d.scheduleOk;
  }
  return out;
}

export function tabErrorCount(e: EditorErrors, tab: EditorTab): number {
  if (tab === 'script') return [e.name, e.body, e.folder, e.budget].filter(Boolean).length;
  if (tab === 'params') return e.params.reduce((n, m) => n + m.length, 0);
  return (e.cron ? 1 : 0) + (e.scheduleInvalid ? 1 : 0);
}
