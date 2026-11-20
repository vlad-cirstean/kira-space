import { t } from '@theme/i18n';
import type { RestoreRow } from '@workbench/restore/types';
import type { DockerRestoreState } from './wire';

/**
 * Dialog rows for the containers a restart stopped. A Compose project is one row: its members start
 * together, in dependency order. `errors` holds each container's last restore failure by id.
 */
export function dockerRestoreRows(
  state: DockerRestoreState,
  errors: ReadonlyMap<string, string>,
  group?: string,
): RestoreRow[] {
  const out: RestoreRow[] = [];
  const projects = new Map<string, RestoreRow>();
  for (const item of state.items) {
    if (item.status === 'back' || item.status === 'gone') {
      out.push({
        id: item.id,
        label: item.name,
        state: 'unavailable',
        status: item.status,
        reason: t(
          item.status === 'back'
            ? 'docker.restore.dialog.back.title'
            : 'docker.restore.dialog.gone.body',
        ),
        group,
      });
      continue;
    }
    const waiting = item.status === 'waiting';
    if (!item.composeProject) {
      out.push({
        id: item.id,
        label: item.name,
        detail: item.image,
        state: waiting ? 'waiting' : 'ready',
        status: item.status,
        reason: waiting ? t('docker.restore.dialog.waiting.label') : undefined,
        error: errors.get(item.id),
        group,
      });
      continue;
    }
    let row = projects.get(item.composeProject);
    if (!row) {
      row = {
        id: `project:${item.composeProject}`,
        label: t('docker.restore.dialog.project.label', { name: item.composeProject }),
        state: 'ready',
        status: 'restorable',
        members: [],
        group,
      };
      projects.set(item.composeProject, row);
      out.push(row);
    }
    row.members?.push({ id: item.id, label: item.name, error: errors.get(item.id) });
    if (waiting) {
      row.state = 'waiting';
      row.status = 'waiting';
      row.reason = t('docker.restore.dialog.waiting.label');
    }
  }
  return out;
}

/** Container ids behind the chosen rows: a project row stands for all its members. */
export function dockerRestoreIds(rows: readonly RestoreRow[], chosen: readonly string[]): string[] {
  const members = new Map(rows.map((r) => [r.id, r.members?.map((m) => m.id) ?? [r.id]]));
  return chosen.flatMap((id) => members.get(id) ?? []);
}
