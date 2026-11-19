import { t } from '@theme/i18n';
import { copyText } from '@workbench/behaviour/clipboard';
import type { MenuItem } from '@workbench/state/contextMenu';

/** One tree or search row's context menu: the worktree is read-only, so nothing here edits. */
export function menuForFilePath(opts: {
  path: string;
  /** The worktree root; Copy path joins it with `path`. */
  worktree: string;
  isDir: boolean;
  expanded?: boolean;
  onToggle?: () => void;
}): MenuItem[] {
  const items: MenuItem[] = [];
  if (opts.isDir && opts.onToggle) {
    items.push({
      type: 'item',
      id: 'toggle',
      label: opts.expanded ? t('common.action.collapse.label') : t('common.action.expand.label'),
      run: opts.onToggle,
    });
  }
  items.push({
    type: 'item',
    id: 'copy-path',
    label: t('common.action.copyPath.label'),
    icon: 'copy',
    run: () => copyText(`${opts.worktree}/${opts.path}`),
  });
  items.push({
    type: 'item',
    id: 'copy-rel-path',
    label: t('space.ade.files.copyRelPath.label'),
    icon: 'copy',
    run: () => copyText(opts.path),
  });
  return items;
}
