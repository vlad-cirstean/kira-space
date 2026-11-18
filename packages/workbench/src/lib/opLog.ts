import type { OpLogRecord, OpStatus } from '@shared/domain/ops';
import { type MessageKey, t } from '@theme/i18n';
import { TONE_TEXT } from '@theme/lib/statusTone';
import { copyAndAnnounce } from '../behaviour/clipboard';
import { monthNames } from '../behaviour/format';
import type { MenuItem } from '../state/contextMenu';

// OpLogPanel.vue's own column contract. `columns` is a plain prop array, so a consumer states its
// column set (order, width, drop priority) in one place and gets the header row for free. Kept out
// of the .vue file so a wrapper's own .ts can import it.
export interface OpLogColumn {
  id: string;
  label: string;
  /** A px grid track, e.g. '64px'. The `command` column always renders as `minmax(240px, 1fr)`. */
  width: string;
  /** Narrow panels drop the highest number first. Absent: never dropped. */
  priority?: number;
  align?: 'end';
}

/** Room the command column keeps; narrower panels drop low-value columns instead. */
export const COMMAND_MIN_PX = 240;
export const COLUMN_GAP_PX = 8;
export const PANEL_PAD_PX = 8;

/** One labelled value in an expanded record. */
export interface OpLogField {
  label: string;
  value: string;
  /** Render in the data font (paths, ids). */
  code?: boolean;
}

/** One recorded step of a multi-step op (Space's git steps). */
export interface OpLogStep {
  argv: string;
  status: OpStatus;
  exitCode: number | null;
  durationMs: number | null;
  stderr: string | null;
}

/** One extra line under an expanded record. */
export interface OpLogDetailLine {
  text: string;
  tone?: 'error';
}

export type OpLogStatusFilter = 'all' | 'running' | 'error';

export const STATUS_ICON: Record<OpStatus, { icon: string; tone: string }> = {
  ok: { icon: 'pass', tone: TONE_TEXT.ok },
  error: { icon: 'error', tone: TONE_TEXT.error },
  cancelled: { icon: 'circle-slash', tone: 'text-muted-foreground' },
  running: { icon: 'loading', tone: 'text-muted-foreground' },
};

const STATUS_LABEL: Record<OpStatus, MessageKey> = {
  running: 'common.state.running.status',
  ok: 'automations.run.state.done.status',
  error: 'workbench.opLog.error.status',
  cancelled: 'common.cancelled.status',
};

export function statusLabel(status: OpStatus): string {
  return t(STATUS_LABEL[status]);
}

function trackPx(width: string): number {
  return Number.parseFloat(width) || 0;
}

/** Drops the highest-`priority` columns (ties: rightmost first) until the fixed tracks leave
 *  `COMMAND_MIN_PX` for the command column. A column without `priority` is never dropped. */
export function fitColumns(
  columns: readonly OpLogColumn[],
  widthPx: number,
  gapPx = COLUMN_GAP_PX,
  paddingPx = PANEL_PAD_PX,
): OpLogColumn[] {
  const kept = [...columns];
  if (widthPx <= 0) return kept;
  const needed = (): number => {
    const fixed = kept.reduce((sum, c) => sum + (c.id === 'command' ? 0 : trackPx(c.width)), 0);
    return fixed + COMMAND_MIN_PX + gapPx * (kept.length - 1) + 2 * paddingPx;
  };
  while (needed() > widthPx) {
    let drop = -1;
    for (let i = kept.length - 1; i >= 0; i--) {
      const priority = kept[i].priority;
      if (priority !== undefined && (drop < 0 || priority > (kept[drop].priority ?? 0))) drop = i;
    }
    if (drop < 0) break;
    kept.splice(drop, 1);
  }
  return kept;
}

/** The CSS grid-template-columns for a fitted column list. */
export function gridTemplateFor(columns: readonly OpLogColumn[]): string {
  return columns
    .map((c) => (c.id === 'command' ? `minmax(${COMMAND_MIN_PX}px, 1fr)` : c.width))
    .join(' ');
}

const pad2 = (n: number): string => String(n).padStart(2, '0');

/** 'HH:MM:SS' for today, 'MMM d HH:MM' for any other day (local time, 24-hour). */
export function formatOpTime(startedAt: string, now: number): string {
  const at = new Date(startedAt);
  const clock = `${pad2(at.getHours())}:${pad2(at.getMinutes())}`;
  if (at.toDateString() === new Date(now).toDateString())
    return `${clock}:${pad2(at.getSeconds())}`;
  return `${monthNames('short')[at.getMonth()]} ${at.getDate()} ${clock}`;
}

// The context-menu items every op-log row gets: Copy command, Copy error, Copy details (only when
// the caller passes `details`), a separator, Cancel. Consumers find items by `id`, never by
// position. `canCancel` false disables Cancel for a running row that cannot be cancelled.
export function opLogMenuItems<R extends OpLogRecord>(
  record: R,
  cancel: () => void,
  canCancel = true,
  copy: (text: string, opts: { message: string }) => Promise<boolean> = (text, { message }) =>
    copyAndAnnounce(text, message),
  details?: string,
): MenuItem[] {
  const items: MenuItem[] = [
    {
      type: 'item',
      id: 'copy-command',
      label: t('common.copyCommand.label'),
      icon: 'copy',
      disabled: !record.command,
      run: () =>
        void copy(record.command ?? '', { message: t('workbench.opLog.copiedCommand.announce') }),
    },
    {
      type: 'item',
      id: 'copy-error',
      label: t('workbench.opLog.copyError.label'),
      icon: 'copy',
      disabled: !record.error,
      run: () =>
        void copy(record.error ?? '', { message: t('workbench.opLog.copiedError.announce') }),
    },
  ];
  if (details !== undefined) {
    items.push({
      type: 'item',
      id: 'copy-details',
      label: t('workbench.opLog.copyDetails.label'),
      icon: 'copy',
      run: () => void copy(details, { message: t('workbench.opLog.copiedDetails.announce') }),
    });
  }
  items.push(
    { type: 'separator' },
    {
      type: 'item',
      id: 'cancel',
      label: t('common.action.cancel.label'),
      icon: 'debug-stop',
      disabled: record.status !== 'running' || !canCancel,
      run: cancel,
    },
  );
  return items;
}
