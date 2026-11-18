import { t } from '@theme/i18n';
import type { MenuItem } from '../state/contextMenu';
import type { FileLinkTarget } from './terminalRenderer';

export type HoveredLink =
  | { kind: 'web'; uri: string }
  | { kind: 'file'; target: FileLinkTarget; abs: string };

export interface LinkMenuActions {
  openLink(uri: string): void;
  openFile(target: FileLinkTarget): void;
  copy(text: string): void;
}

/** Right-click menu of a hovered terminal link. Copy path copies the resolved absolute path. */
export function linkMenuItems(link: HoveredLink, actions: LinkMenuActions): MenuItem[] {
  if (link.kind === 'web') {
    return [
      {
        type: 'item',
        id: 'open-link',
        label: t('common.action.open.label'),
        icon: 'external-link',
        run: () => actions.openLink(link.uri),
      },
      {
        type: 'item',
        id: 'copy-link',
        label: t('common.action.copyLink.label'),
        icon: 'copy',
        run: () => actions.copy(link.uri),
      },
    ];
  }
  return [
    {
      type: 'item',
      id: 'open-file',
      label: t('common.action.open.label'),
      icon: 'file',
      run: () => actions.openFile(link.target),
    },
    {
      type: 'item',
      id: 'copy-path',
      label: t('common.action.copyPath.label'),
      icon: 'copy',
      run: () => actions.copy(link.abs),
    },
  ];
}
