import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { errorText } from '@workbench/bridge/codedError';
import { type MenuItem, useContextMenuStore } from '@workbench/state/contextMenu';
import { reactive } from 'vue';
import { useLaunchOpener } from '../dialog/useLaunchOpener';
import { useCloseSession, useRestartSession } from '../queries';
import { useAdeBoardUiStore } from '../state/adeBoardUi';
import { useAdePanelTabsStore } from '../state/adePanelTabs';
import { useAdeTakeOverStore } from '../state/adeTakeOver';
import type { SessionView } from './sessionView';

export interface SessionMenuOptions {
  /** Session of an archived task: Resume replaces Take over. */
  archived?: boolean;
  /** Row-local items, such as the log toggle; placed before Copy session ID. */
  extra?: MenuItem[];
}

/** Menu of a session tab or dropdown row: Take over / Resume (stopped), Restart and Close
 *  (running), the caller's `extra` items, Copy session ID. */
export function useSessionMenu(): {
  open: (ev: MouseEvent, view: SessionView, opts?: SessionMenuOptions) => void;
  openFor: (el: Element, view: SessionView, opts?: SessionMenuOptions) => void;
  busy: Set<string>;
} {
  const ui = useAdeBoardUiStore();
  const tabs = useAdePanelTabsStore();
  const contextMenu = useContextMenuStore();
  const launchOpener = useLaunchOpener();
  const restartM = useRestartSession();
  const closeM = useCloseSession();
  const takeOver = useAdeTakeOverStore();
  const { copy } = useCopy();
  const busy = reactive(new Set<string>());

  async function restart(view: SessionView): Promise<void> {
    const { id, taskId } = view.session;
    delete ui.actionError[taskId];
    busy.add(id);
    try {
      const launch = await restartM.mutateAsync({ sessionId: id });
      if (launch.terminalId) await launchOpener.open(launch);
      if (launch.sessionId) tabs.selectSession(taskId, launch.sessionId);
    } catch (err) {
      ui.actionError[taskId] = errorText(err);
    } finally {
      busy.delete(id);
    }
  }

  async function close(view: SessionView): Promise<void> {
    const { id, taskId } = view.session;
    delete ui.actionError[taskId];
    busy.add(id);
    try {
      await closeM.mutateAsync({ sessionId: id });
    } catch (err) {
      ui.actionError[taskId] = errorText(err);
    } finally {
      busy.delete(id);
    }
  }

  function items(view: SessionView, opts: SessionMenuOptions = {}): MenuItem[] {
    const { id, taskId, agentSessionId } = view.session;
    const list: MenuItem[] = [];
    if (view.session.state === 'running') {
      const noConversation = view.headless && !agentSessionId;
      list.push(
        {
          type: 'item',
          id: 'ade-session-restart',
          label: t('space.ade.sessionTab.restart.label'),
          icon: 'debug-restart',
          disabled: busy.has(id) || noConversation,
          hint: noConversation ? t('space.ade.sessionList.waitingId.status') : undefined,
          run: () => restart(view),
        },
        {
          type: 'item',
          id: 'ade-session-close',
          label: t('space.ade.sessionTab.close.label'),
          icon: 'close',
          disabled: busy.has(id),
          run: () => close(view),
        },
      );
    } else if (opts.archived) {
      if (agentSessionId) {
        list.push({
          type: 'item',
          id: 'ade-session-resume-menu',
          label: t('common.action.resume.label'),
          icon: 'debug-continue',
          disabled: takeOver.pending.has(id),
          run: () => takeOver.resumeArchived(taskId, id),
        });
      }
    } else {
      list.push({
        type: 'item',
        id: 'ade-session-takeover-menu',
        label: t('common.action.takeOver.label'),
        icon: 'debug-continue',
        disabled: takeOver.pending.has(id),
        run: () => takeOver.request(id),
      });
    }
    if (opts.extra?.length) list.push(...opts.extra);
    if (list.length) list.push({ type: 'separator' });
    list.push({
      type: 'item',
      id: 'ade-session-copy-id',
      label: t('space.ade.sessionTab.copyId.label'),
      icon: 'copy',
      disabled: !agentSessionId,
      hint: agentSessionId ? undefined : t('space.ade.sessionList.waitingId.status'),
      run: () => {
        void copy(agentSessionId);
      },
    });
    return list;
  }

  function open(ev: MouseEvent, view: SessionView, opts?: SessionMenuOptions): void {
    ev.preventDefault();
    contextMenu.openContextMenu(ev, items(view, opts));
  }

  function openFor(el: Element, view: SessionView, opts?: SessionMenuOptions): void {
    contextMenu.openContextMenuFor(el, items(view, opts));
  }

  return { open, openFor, busy };
}
