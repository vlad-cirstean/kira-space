<script setup lang="ts">
import { needsKindLabel } from '@ade/board/needsYou';
import { THEME_TONE } from '@ade/tones';
import EmptyState from '@theme/components/EmptyState.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Alert } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { TONE_BADGE } from '@theme/lib/statusTone';
import { useApprovals } from '@workbench/approvals/approvalsQueries';
import { formatRelative } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import ConfirmDialogView from '@workbench/components/ConfirmDialogView.vue';
import QueryState from '@workbench/components/QueryState.vue';
import { storeToRefs } from 'pinia';
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import PermissionHint from '../components/PermissionHint.vue';
import PhoneApprovals from '../components/PhoneApprovals.vue';
import ReplySheet from '../components/ReplySheet.vue';
import { useAuthStore } from '../state/auth';
import { newIntentKey, useAdeWrites } from '../state/useAdeWrites';
import { useAgentsModel } from '../state/useAgentsModel';

// Everything that waits on you, most urgent first. An agent waiting in its Claude Code session
// can be answered from here, or taken over when a run is stuck.
const { model, boardQuery } = useAgentsModel();
const { permissions } = storeToRefs(useAuthStore());
const { data: approvals } = useApprovals();
const writes = useAdeWrites();
const router = useRouter();

const replying = ref<{ sessionId: string; context: string } | null>(null);
const takingOver = ref<{ sessionId: string; what: string } | null>(null);
const takeOverError = ref('');
let takeOverKey = newIntentKey();

const canReply = computed(() => permissions.value.agentInput);
const needsAgentInput = computed(
  () =>
    !canReply.value &&
    !!model.value?.needs.items.some(
      (n) => n.sessionId && (n.kind === 'question' || n.kind === 'stuck run'),
    ),
);

function askTakeOver(sessionId: string, what: string): void {
  takeOverError.value = '';
  takeOverKey = newIntentKey();
  takingOver.value = { sessionId, what };
}

async function confirmTakeOver(): Promise<void> {
  const target = takingOver.value;
  if (!target) return;
  takeOverError.value = '';
  try {
    const launch = await writes.takeOver.mutateAsync({
      sessionId: target.sessionId,
      key: takeOverKey,
    });
    takingOver.value = null;
    void router.push({ name: 'terminal', params: { sessionId: launch.sessionId } });
  } catch (err) {
    takeOverError.value = errorText(err);
  }
}
</script>

<template>
  <section class="flex flex-col" data-testid="needs-screen">
    <PhoneApprovals />
    <QueryState :query="boardQuery" :is-empty="() => false">
      <template #error>
        <Alert variant="error" strip data-testid="needs-error">{{
          t('mobile.needs.loadError.error')
        }}</Alert>
      </template>
      <template v-if="model">
        <EmptyState
          v-if="model.needs.empty && !approvals?.length"
          fill
          icon="check"
          data-testid="needs-empty"
          :title="t('mobile.needs.list.empty')"
          :description="model.needs.footer"
        />
        <template v-else-if="!model.needs.empty">
          <PermissionHint v-if="needsAgentInput" :what="t('mobile.needs.replyOff.hint')" />
          <ul class="m-0 flex list-none flex-col p-0">
            <li
              v-for="item in model.needs.items"
              :key="item.id"
              class="flex flex-col gap-1 border-b border-border px-3 py-3"
              data-testid="needs-item"
              :data-kind="item.kind"
            >
              <span class="flex items-center gap-2">
                <Badge :variant="TONE_BADGE[THEME_TONE[item.tone]]">{{
                  needsKindLabel(item.kind)
                }}</Badge>
                <span class="text-kira-sm text-subtle">{{
                  item.ageMs === null ? '' : formatRelative(0, item.ageMs)
                }}</span>
                <span class="ml-auto flex min-w-0 items-center gap-1.5 text-kira-sm text-subtle">
                  <StatusDot :color="model.cards.get(item.taskId)?.color" />
                  <span class="truncate">{{ model.cards.get(item.taskId)?.title }}</span>
                </span>
              </span>
              <span class="text-kira-md">{{ item.what }}</span>
              <span v-if="item.detail" class="text-kira-sm text-muted-foreground">{{
                item.detail
              }}</span>
              <span v-if="item.scope" class="text-kira-sm text-subtle">{{ item.scope }}</span>
              <span v-if="canReply && item.sessionId" class="flex gap-2 pt-1">
                <Button
                  size="touch"
                  v-if="item.kind === 'question'"
                  variant="dialog-primary"
                  data-testid="needs-reply"
                  @click="replying = { sessionId: item.sessionId, context: item.what }"
                >
                  {{ t('space.common.reply.label') }}
                </Button>
                <Button
                  size="touch"
                  v-if="item.kind === 'question'"
                  as-child
                  variant="dialog"
                  class="no-underline"
                >
                  <RouterLink
                    :to="{ name: 'terminal', params: { sessionId: item.sessionId } }"
                    data-testid="needs-terminal"
                  >
                    {{ t('common.module.terminal.label') }}
                  </RouterLink>
                </Button>
                <Button
                  size="touch"
                  v-else-if="item.kind === 'stuck run'"
                  variant="dialog"
                  data-testid="needs-take-over"
                  @click="askTakeOver(item.sessionId, item.what)"
                >
                  {{ t('common.action.takeOver.label') }}
                </Button>
              </span>
            </li>
          </ul>
          <FieldDescription class="px-3 py-2" data-testid="needs-footer">{{
            model.needs.footer
          }}</FieldDescription>
        </template>
      </template>
    </QueryState>
    <ReplySheet
      :open="replying !== null"
      :session-id="replying?.sessionId ?? ''"
      :context="replying?.context ?? ''"
      @close="replying = null"
    />
    <ConfirmDialogView
      touch
      tone="primary"
      :open="takingOver !== null"
      :title="t('space.common.takeOverConfirm.title')"
      :message="t('mobile.needs.takeOverConfirm.body', { what: takingOver?.what ?? '' })"
      :confirm-label="t('common.action.takeOver.label')"
      :pending="writes.takeOver.isPending.value"
      :error="takeOverError"
      :testids="{
        root: 'confirm-dialog',
        cancel: 'confirm-cancel',
        confirm: 'confirm-yes',
        error: 'confirm-error',
      }"
      @cancel="takingOver = null"
      @confirm="confirmTakeOver"
    />
  </section>
</template>
