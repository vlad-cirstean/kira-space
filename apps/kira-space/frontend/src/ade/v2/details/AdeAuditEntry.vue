<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { formatDuration, formatRelative } from '@workbench/behaviour/format';
import { computed, ref } from 'vue';
import AdeRow from '../panel/AdeRow.vue';
import type { DbAuditApproval, DbAuditEntry } from '../wire';
import AdeOriginSession from './AdeOriginSession.vue';

// One Kira DB action of an agent session in the Details list, with the approvals it raised.
// Read-only. The statement has its literals masked by the backend; nothing here can show a value.
const props = defineProps<{ entry: DbAuditEntry; taskId: string; archived?: boolean }>();
const emit = defineEmits<{ openSession: [sessionId: string] }>();

const expanded = ref(false);
const STATUS_VARIANT = {
  running: 'info',
  ok: 'ok',
  error: 'error',
  refused: 'warn',
  cancelled: 'default',
} as const;
const OUTCOME_VARIANT = {
  approved: 'ok',
  denied: 'error',
  pending: 'info',
} as const;

const hasStatement = computed(() => props.entry.statement !== '');
function statusLabel(status: DbAuditEntry['status']): string {
  switch (status) {
    case 'running':
      return t('common.state.running.status');
    case 'ok':
      return t('common.done.status');
    case 'error':
      return t('common.state.failed.status');
    case 'refused':
      return t('space.ade.details.dbAudit.refused.status');
    case 'cancelled':
      return t('common.cancelled.status');
  }
}
function outcomeLabel(outcome: DbAuditApproval['outcome']): string {
  switch (outcome) {
    case 'approved':
      return t('space.ade.details.dbAudit.accepted.status');
    case 'denied':
      return t('space.ade.details.dbAudit.denied.status');
    case 'timedOut':
      return t('space.ade.details.dbAudit.timedOut.status');
    case 'pending':
      return t('space.common.pending.status');
    default:
      return t('space.ade.details.dbAudit.unanswered.status');
  }
}
function subjectLabel(a: DbAuditApproval): string {
  switch (a.subject) {
    case 'db.query':
      return t('space.ade.details.dbAudit.subjectQuery.label');
    case 'db.response':
      return t('space.ade.details.dbAudit.subjectResponse.label');
    case 'db.exemption':
      return t('space.ade.details.dbAudit.subjectExemption.label');
    case 'kube.read':
      return t('space.ade.details.dbAudit.subjectKube.label');
    case 'script.run':
      return t('space.ade.details.dbAudit.subjectScript.label');
    case 'api.request':
      return t('space.ade.details.dbAudit.subjectApi.label');
    default:
      return a.subject;
  }
}
const outcomeVariant = (a: DbAuditApproval): 'ok' | 'error' | 'info' | 'warn' =>
  a.outcome in OUTCOME_VARIANT
    ? OUTCOME_VARIANT[a.outcome as keyof typeof OUTCOME_VARIANT]
    : 'warn';
</script>

<template>
  <AdeRow
    as="div"
    :label="formatRelative(entry.startedAt)"
    data-testid="ade-details-dbaudit-entry"
    data-kind="audit"
    :data-status="entry.status"
  >
    <template #lead><CodiconIcon name="database" :size="13" /></template>
    <div class="flex min-w-0 flex-col gap-1">
      <div class="flex min-w-0 items-center gap-2">
        <span class="font-data">{{ entry.tool }}</span>
        <span v-if="entry.connectionName" class="min-w-0 truncate text-muted-foreground">{{
          entry.connectionName
        }}</span>
        <Badge :variant="STATUS_VARIANT[entry.status]">
          <CodiconIcon v-if="entry.status === 'running'" name="loading" spin :size="12" />
          {{ statusLabel(entry.status) }}
        </Badge>
        <span v-if="entry.durationMs !== null" class="text-subtle">{{
          formatDuration(entry.durationMs)
        }}</span>
      </div>
      <InlineState
        v-if="entry.error"
        kind="error"
        class="p-0"
        data-testid="ade-details-dbaudit-error"
        >{{
          entry.error
        }}</InlineState
      >
      <div v-if="hasStatement" class="flex flex-col items-start gap-1">
        <pre
          class="m-0 max-w-full whitespace-pre-wrap break-all font-data text-kira-sm"
          :class="expanded ? '' : 'line-clamp-2'"
          data-testid="ade-details-dbaudit-statement"
        >{{ entry.statement }}{{ entry.statementTruncated ? '…' : '' }}</pre>
        <Button
          variant="link"
          size="inline"
          data-testid="ade-details-dbaudit-expand"
          @click="expanded = !expanded"
          >{{
            expanded ? t('common.action.showLess.label') : t('common.action.showAll.label')
          }}</Button
        >
      </div>
      <div
        v-else-if="entry.statementWithheld"
        class="text-subtle"
        data-testid="ade-details-dbaudit-withheld"
        >{{
          t('space.ade.details.dbAudit.withheld.body', { count: entry.statementLen })
        }}</div
      >
      <div
        v-else-if="entry.summary"
        class="truncate font-data text-kira-sm text-muted-foreground"
        >{{
          entry.summary
        }}</div
      >
      <div
        v-for="a in entry.approvals"
        :key="a.id"
        class="flex flex-col gap-0.5"
        data-testid="ade-details-dbaudit-approval"
        :data-outcome="a.outcome"
        :data-subject="a.subject"
      >
        <div class="flex items-center gap-2">
          <CodiconIcon name="shield" :size="12" />
          <Badge v-if="a.subject === 'db.exemption'">{{ subjectLabel(a) }}</Badge>
          <span v-else>{{ subjectLabel(a) }}</span>
          <Badge :variant="outcomeVariant(a)">{{ outcomeLabel(a.outcome) }}</Badge>
        </div>
        <template v-if="a.subject === 'db.exemption'">
          <div v-if="a.exemptionLimits.length" class="text-subtle">{{
            t('space.ade.details.dbAudit.limits.label', {
              limits: a.exemptionLimits.map((l) => `${l.name} ${l.threshold}`).join(', '),
            })
          }}</div>
          <div v-if="a.exemptionReason" class="text-subtle">{{
            t('space.ade.details.dbAudit.reason.label', { reason: a.exemptionReason })
          }}</div>
        </template>
      </div>
    </div>
    <template #actions>
      <AdeOriginSession
        :task-id="taskId"
        :session-id="entry.sessionId"
        :archived="archived"
        @open="emit('openSession', $event)"
      />
    </template>
  </AdeRow>
</template>
