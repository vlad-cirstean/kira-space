import { computed, watch } from 'vue';
import { useOpsStore } from '../../../state/ops';
import type { SpaceOpRecord } from '../../../state/opsDomain';
import { type AdeOpKind, useAdeOpsStore } from './adeOps';

// The backend's agent ops (category `agent`, from the MCP tools and their approval queue) and the
// `mcp` git tools that touch a remote, each with a task id. Backend kind to client kind; the one
// mapping. A tool renamed or a new agent op worth showing is one row here.
const AGENT_KINDS: Record<string, AdeOpKind> = {
  agentApproval: 'agentApproval',
  agentBranches: 'agentBranches',
  agentRename: 'agentRename',
  agentChange: 'agentChange',
};
const MCP_KINDS: Record<string, AdeOpKind> = {
  fetch_remote: 'agentFetch',
  push_branch: 'agentPush',
};

function mirrorKind(rec: SpaceOpRecord): AdeOpKind | null {
  if (!rec.taskId) return null;
  if (rec.category === 'agent') return AGENT_KINDS[rec.kind] ?? null;
  if (rec.category === 'mcp' && rec.module === 'ade') return MCP_KINDS[rec.kind] ?? null;
  return null;
}

/**
 * Mirrors running backend agent ops into `adeOps`, so task cards show a chip and a slow finish toasts.
 * Ops already finished at hydration never track, so a restart replays no toast.
 */
export function installAdeOpMirror(): () => void {
  const log = useOpsStore();
  const ops = useAdeOpsStore();
  const tracked = new Set<string>();
  const mirrored = computed(() => log.records.filter((r) => mirrorKind(r) !== null));
  return watch(
    mirrored,
    (records) => {
      const live = new Set<string>();
      for (const rec of records) {
        const kind = mirrorKind(rec);
        const id = `be:${rec.id}`;
        if (!kind) continue;
        live.add(id);
        if (rec.status === 'running') {
          if (tracked.has(id)) continue;
          tracked.add(id);
          ops.track({ id, kind, taskId: rec.taskId, name: rec.subject });
        } else if (tracked.delete(id)) {
          ops.settle(id, {
            ok: rec.status === 'ok',
            error: rec.error ?? undefined,
            durationMs: rec.durationMs ?? 0,
          });
        }
      }
      // A cleared log drops a running op's record: end its chip without a toast.
      for (const id of tracked) {
        if (live.has(id)) continue;
        tracked.delete(id);
        ops.settle(id, { ok: true, durationMs: 0 });
      }
    },
    { immediate: true },
  );
}
