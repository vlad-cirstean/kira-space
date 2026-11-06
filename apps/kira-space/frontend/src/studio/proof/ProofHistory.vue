<script setup lang="ts">
import type { ProofSummary } from '@shared/domain/proof';
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Badge } from '@theme/components/ui/badge';
import { formatDateTime } from '@theme/format';
import { t } from '@theme/i18n';
import { rowActions, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import QueryState from '@workbench/components/QueryState.vue';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { useContextMenuStore } from '@workbench/state/contextMenu';
import { control } from '../bridge/control';
import { useDeleteProof, useProofSessions } from './queries';
import { useProofWindowStore } from './state';

// The recent validation sessions, newest first, 30 at most (the backend keeps no more).
const store = useProofWindowStore();
const sessions = useProofSessions();
const remove = useDeleteProof();
const confirmDialog = useConfirmDialogStore();
const contextMenu = useContextMenuStore();

function open(session: ProofSummary): void {
  store.selectSession(session.id);
  if (!session.seen) fireAndForget(control.proofMarkSeen(session.id), 'proof.markSeen');
}

async function askDelete(session: ProofSummary): Promise<void> {
  await confirmDialog.confirm({
    title: t('studio.proof.delete.title'),
    message: t('studio.proof.delete.body', { name: session.name }),
    confirmLabel: t('common.action.delete.label'),
    tone: 'danger',
    run: () => remove.mutateAsync(session.id),
  });
}

function onContextMenu(ev: MouseEvent, session: ProofSummary): void {
  ev.preventDefault();
  contextMenu.openContextMenu(ev, [
    { type: 'item', id: 'open', label: t('common.action.open.label'), run: () => open(session) },
    {
      type: 'item',
      id: 'delete',
      label: t('common.action.delete.label'),
      icon: 'trash',
      danger: true,
      run: () => askDelete(session),
    },
  ]);
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col" data-testid="proof-history">
    <div
      class="flex h-tabbar shrink-0 items-center bg-chrome px-3 text-kira-sm font-medium text-muted-foreground"
    >
      {{ t('common.module.history.label') }}
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto p-1">
      <QueryState :query="sessions" variant="inline">
        <template #empty>
          <p class="m-0 p-2 text-muted-foreground" data-testid="proof-history-empty">
            {{ t('studio.proof.history.empty') }}
          </p>
        </template>
        <template #default="{ data }">
          <ul class="m-0 list-none p-0" :aria-label="t('studio.proof.history.aria')">
            <li v-for="session in data" :key="session.id" class="group relative">
              <button
                type="button"
                :class="
                  cn(
                    rowVariants({ layout: 'double', selected: store.sessionId === session.id }),
                    'w-full flex-col gap-0.5 rounded-kira-sm border-0 bg-transparent py-1.5 pl-2 text-left',
                  )
                "
                :aria-current="store.sessionId === session.id"
                :data-testid="`proof-session-${session.id}`"
                @click="open(session)"
                @contextmenu="onContextMenu($event, session)"
              >
                <span class="flex min-w-0 items-center gap-1.5">
                  <StatusDot
                    v-if="!session.seen"
                    tone="info"
                    :label="t('studio.proof.history.unseen.aria')"
                    data-testid="proof-session-unseen"
                  />
                  <span class="min-w-0 truncate" data-testid="proof-session-name">{{
                    session.name
                  }}</span>
                  <Badge
                    v-if="session.pending"
                    variant="warn"
                    data-testid="proof-session-pending"
                    >{{
                      t('studio.proof.pending.label')
                    }}</Badge
                  >
                </span>
                <span :class="cn(rowMeta, 'flex items-center gap-1.5')">
                  <CodiconIcon name="history" :size="12" />
                  <span>{{ formatDateTime(session.closedAt) }}</span>
                  <span>·</span>
                  <span>{{ t('studio.proof.session.steps.label', { count: session.steps }) }}</span>
                </span>
              </button>
              <TooltipIconButton
                icon="trash"
                :label="t('common.action.delete.label')"
                variant="toolbar"
                size="kira-icon"
                :class="cn(rowActions, 'absolute right-1 top-1')"
                :data-testid="`proof-session-delete-${session.id}`"
                @click="askDelete(session)"
              />
            </li>
          </ul>
        </template>
      </QueryState>
    </div>
  </div>
</template>
