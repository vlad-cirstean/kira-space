import type { ApprovalItem } from '@shared/domain/approvals';
import { connectionKindSchema } from '@shared/domain/connection';
import {
  apiRequestPayloadSchema,
  type DbMcpApprovalPlan,
  dbExemptionPayloadSchema,
  dbQueryPayloadSchema,
  dbResponsePayloadSchema,
  kubeReadPayloadSchema,
} from '@shared/domain/dbmcp';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { type MessageKey, t } from '@theme/i18n';
import type { ApprovalKinds, ApprovalRisk } from '@workbench/approvals/registry';
import { formatCount, formatDuration } from '@workbench/behaviour/format';
import { markRaw } from 'vue';
import { KIND_LABEL } from '../project/connection/kinds';
import ApiRequestBody from './ApiRequestBody.vue';
import ApiRequestDetails from './ApiRequestDetails.vue';
import DbExemptionBody from './DbExemptionBody.vue';
import DbExemptionDetails from './DbExemptionDetails.vue';
import DbQueryBody from './DbQueryBody.vue';
import DbResponseBody from './DbResponseBody.vue';
import DbResponseDetails from './DbResponseDetails.vue';
import KubeReadBody from './KubeReadBody.vue';
import ProofOpenWindow from './ProofOpenWindow.vue';
import ProofReviewBody from './ProofReviewBody.vue';
import ProofReviewDetails from './ProofReviewDetails.vue';
import ReviewResultOption from './ReviewResultOption.vue';
import ScriptRunBody from './ScriptRunBody.vue';
import ScriptRunDetails from './ScriptRunDetails.vue';

const DB_CLASS_LABEL: Record<string, MessageKey> = {
  read: 'studio.project.connectionDialog.mcp.read.label',
  write: 'studio.project.connectionDialog.mcp.write.label',
  ddl: 'studio.project.connectionDialog.mcp.ddl.label',
  unknown: 'studio.dbMcp.approval.target.unclassified.label',
};

const KUBE_OPERATION_LABEL = {
  discover: 'kubernetes.mcp.discover.label',
  list: 'kubernetes.mcp.list.label',
  describe: 'kubernetes.detail.tab.describe.label',
  logs: 'docker.tab.logs.label',
} as const satisfies Record<string, MessageKey>;

const RANK: Record<string, number> = { read: 0, write: 1, ddl: 2 };
const MAX_PLAN_ISSUES = 2;

function dbOperation(item: ApprovalItem): string {
  const key = DB_CLASS_LABEL[item.operation.class];
  return key ? t(key) : item.operation.class;
}

// "PostgreSQL · db.internal · shop / public": the connection's kind label, then its location.
function connectionWhere(item: ApprovalItem): string {
  const kind = connectionKindSchema.safeParse(item.target.kind).data;
  return [kind ? KIND_LABEL[kind] : item.target.kind, item.target.location]
    .filter(Boolean)
    .join(' · ');
}

function rowsRisk(estimated: number): ApprovalRisk {
  return {
    text: t('studio.dbMcp.approval.risk.rows.status', { rows: formatCount(estimated) }),
    tone: 'warn',
  };
}

// The plan's estimate over its threshold, then its issues: two tags, the rest as "+N more".
function planRisks(plan: DbMcpApprovalPlan | null, withRows: boolean): ApprovalRisk[] {
  if (!plan) return [];
  const out: ApprovalRisk[] = [];
  if (withRows && plan.estimatedRowsRead !== null && plan.estimatedRowsRead > plan.thresholdRows) {
    out.push({ ...rowsRisk(plan.estimatedRowsRead), testid: 'db-mcp-approval-plan-rows' });
  }
  for (const issue of plan.issues.slice(0, MAX_PLAN_ISSUES)) {
    out.push({
      text: issue.severity,
      tone: 'warn',
      tooltip: issue.message,
      testid: 'db-mcp-approval-plan-issue',
    });
  }
  const rest = plan.issues.length - MAX_PLAN_ISSUES + plan.issuesOmitted;
  if (rest > 0) {
    out.push({ text: t('common.more.label', { count: rest }), tone: 'warn' });
  }
  return out;
}

function queryRisks(item: ApprovalItem): ApprovalRisk[] {
  const p = dbQueryPayloadSchema.parse(item.payload);
  const out = planRisks(p.plan, true);
  // The statement looks less powerful than the mode the agent declared.
  if ((RANK[p.detected] ?? Infinity) < (RANK[p.mode] ?? Infinity)) {
    out.push({
      text: t('studio.dbMcp.approval.detected.label', { class: p.detected }),
      tone: 'warn',
      testid: 'db-mcp-approval-declared-higher',
    });
  }
  return out;
}

function exemptionRisks(item: ApprovalItem): ApprovalRisk[] {
  const p = dbExemptionPayloadSchema.parse(item.payload);
  const out: ApprovalRisk[] = p.limits.map((l) => {
    const show = l.unit === 'ms' ? formatDuration : formatCount;
    const text =
      l.limit === 'statement_timeout'
        ? t('studio.dbMcp.approval.risk.timeout.status', {
            measured: show(l.measured),
            max: show(l.max),
          })
        : t('studio.dbMcp.approval.risk.rows.status', { rows: show(l.measured) });
    return {
      text,
      tone: 'warn',
      testid: 'db-mcp-exemption-limit',
      attrs: { 'data-limit': l.limit },
    };
  });
  // Each datum once: the plan's estimate only when no expensive_query limit already says it.
  const hasRowsLimit = p.limits.some((l) => l.limit === 'expensive_query');
  return [...out, ...planRisks(p.plan, !hasRowsLimit)];
}

function isRead(method: string): boolean {
  return ['GET', 'HEAD', 'OPTIONS'].includes(method);
}

function apiRisks(item: ApprovalItem): ApprovalRisk[] {
  const p = apiRequestPayloadSchema.parse(item.payload);
  const out: ApprovalRisk[] = [];
  if (!isRead(p.method) && p.credentials.length > 0) {
    out.push({
      text: t('studio.dbMcp.approval.risk.credentialSent.status'),
      tone: 'warn',
      testid: 'api-approval-credential',
      tooltip: p.credentials
        .map((c) =>
          t('studio.dbMcp.approval.api.credential.label', {
            name: c.name,
            hosts: c.hosts.join(', '),
          }),
        )
        .join('\n'),
    });
  }
  if (p.body?.truncated) {
    out.push({
      text: t('studio.dbMcp.approval.risk.bodyTruncated.status'),
      tone: 'warn',
      testid: 'api-approval-body-truncated',
    });
  }
  return out;
}

// P520: Studio's approval kinds (internal/approvals Kind* constants). Each maps a subject to its
// renderers; the shared surface supplies the chrome.
export function studioApprovalKinds(): ApprovalKinds {
  return {
    'db.query': {
      content: markRaw(DbQueryBody),
      footerAction: markRaw(ReviewResultOption),
      operation: (item) =>
        dbQueryPayloadSchema.parse(item.payload).mode === 'masked'
          ? t('studio.dbMcp.approval.op.maskedRead.label')
          : dbOperation(item),
      where: connectionWhere,
      risks: queryRisks,
    },
    'db.exemption': {
      content: markRaw(DbExemptionBody),
      details: markRaw(DbExemptionDetails),
      footerAction: markRaw(ReviewResultOption),
      operation: () => t('studio.dbMcp.approval.op.liftLimit.label'),
      where: connectionWhere,
      risks: exemptionRisks,
      labels: () => ({
        approve: t('studio.dbMcp.approval.exemption.approve.label'),
        deny: t('studio.dbMcp.approval.exemption.deny.label'),
      }),
    },
    'db.response': {
      content: markRaw(DbResponseBody),
      details: markRaw(DbResponseDetails),
      operation: () => t('common.field.result.label'),
      where: connectionWhere,
      risks: (item) =>
        dbResponsePayloadSchema.parse(item.payload).alreadyRan
          ? [
              {
                text: t('studio.dbMcp.approval.risk.alreadyRan.status'),
                tone: 'warn',
                testid: 'db-mcp-preview-ran',
              },
            ]
          : [],
      labels: () => ({
        approve: t('studio.dbMcp.preview.send.label'),
        deny: t('studio.dbMcp.preview.withhold.label'),
      }),
    },
    'kube.read': {
      content: markRaw(KubeReadBody),
      operation: (item) =>
        t(KUBE_OPERATION_LABEL[kubeReadPayloadSchema.parse(item.payload).operation]),
      // The location already ends in the namespace when one is set.
      where: (item) =>
        [
          item.target.location,
          kubeReadPayloadSchema.parse(item.payload).namespace
            ? ''
            : t('kubernetes.namespace.all.option'),
        ]
          .filter(Boolean)
          .join(' · '),
    },
    'script.run': {
      content: markRaw(ScriptRunBody),
      details: markRaw(ScriptRunDetails),
      operation: () => t('automations.dialog.run.untitled.title'),
      where: (item) => item.target.location,
      whereData: true,
    },
    'api.request': {
      content: markRaw(ApiRequestBody),
      details: markRaw(ApiRequestDetails),
      operation: (item) => apiRequestPayloadSchema.parse(item.payload).method,
      where: (item) =>
        apiRequestPayloadSchema.parse(item.payload).environment?.name ??
        t('studio.api.environments.none.option'),
      targetData: true,
      risks: apiRisks,
      labels: () => ({
        approve: t('common.action.send.label'),
        deny: t('studio.dbMcp.approval.api.dontSend.label'),
      }),
    },
    'proof.review': {
      content: markRaw(ProofReviewBody),
      details: markRaw(ProofReviewDetails),
      footerAction: markRaw(ProofOpenWindow),
      operation: () => t('common.action.edit.label'),
      where: (item) =>
        t('studio.proof.approval.rev.label', {
          n: proofReviewPayloadSchema.parse(item.payload).revision,
        }),
      requester: () => '',
      labels: () => ({
        approve: t('studio.proof.change.accept.label'),
        deny: t('studio.proof.change.reject.label'),
      }),
    },
  };
}
