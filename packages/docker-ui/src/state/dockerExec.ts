import { newUuid } from '@shared/ids';
import { t } from '@theme/i18n';
import { cleanupTabRuntime } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import type { DockerContext } from '../context';

export interface DockerExecSession {
  id: string;
  containerId: string;
  title: string;
  n: number;
}

export const useDockerExecSessionsStore = defineStore('dockerExecSessions', () => {
  const sessions = reactive<DockerExecSession[]>([]);

  function forContainer(containerId: string): DockerExecSession[] {
    return sessions.filter((s) => s.containerId === containerId);
  }

  function open(ctx: DockerContext, containerId: string): DockerExecSession {
    const n = Math.max(0, ...forContainer(containerId).map((s) => s.n)) + 1;
    const session: DockerExecSession = {
      id: `docker-exec:${newUuid()}`,
      containerId,
      title: t('docker.exec.session.title', { n }),
      n,
    };
    ctx.execContainers.set(session.id, containerId);
    sessions.push(session);
    return session;
  }

  function close(ctx: DockerContext, id: string): void {
    const idx = sessions.findIndex((s) => s.id === id);
    if (idx === -1) return;
    sessions.splice(idx, 1);
    ctx.execStore.closeTerminalSession(id);
    cleanupTabRuntime(id);
    ctx.execContainers.delete(id);
  }

  function closeForContainer(ctx: DockerContext, containerId: string): void {
    for (const s of forContainer(containerId)) close(ctx, s.id);
  }

  return { sessions, forContainer, open, close, closeForContainer };
});
