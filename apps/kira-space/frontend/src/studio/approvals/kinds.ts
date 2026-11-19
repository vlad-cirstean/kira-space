import type { ApprovalItem } from '@shared/domain/approvals';
import { connectionKindSchema } from '@shared/domain/connection';
import {
  apiRequestPayloadSchema,
  apiResponsePayloadSchema,
  type DbMcpApprovalPlan,
  type DbQueryPayload,
  dbExemptionPayloadSchema,
  dbQueryPayloadSchema,
  dbResponsePayloadSchema,
  kubeReadPayloadSchema,
} from '@shared/domain/dbmcp';
import type { KeptUse } from '@shared/domain/kept';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { type MessageKey, t } from '@theme/i18n';
import type { ApprovalKinds, ApprovalRisk } from '@workbench/approvals/registry';
import { formatCount, formatDuration } from '@workbench/behaviour/format';
import { markRaw } from 'vue';
import { KIND_LABEL } from '../project/connection/kinds';
import ApiRequestBody from './ApiRequestBody.vue';
import ApiRequestDetails from './ApiRequestDetails.vue';
import ApiResponseBody from './ApiResponseBody.vue';
import ApiResponseDetails from './ApiResponseDetails.vue';
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
  logs: 'common.tab.logs.label',
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
  const out = [
    ...planRisks(p.plan, true),
    ...keptRisks(p.kept, item.expiresAt, 'db-approval'),
    ...queryScriptRisks(p.script),
  ];
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
  return [
    ...out,
    ...planRisks(p.plan, !hasRowsLimit),
    ...keptRisks(p.kept, item.expiresAt, 'db-approval'),
    ...queryScriptRisks(p.script),
  ];
}

// The tags for a run_query result script: what it may keep and that the agent sees its output, not the rows.
function queryScriptRisks(script: DbQueryPayload['script']): ApprovalRisk[] {
  if (!script) return [];
  const out: ApprovalRisk[] = [];
  if (script.mayKeep.length) {
    out.push({
      text: t('studio.kept.risk.keeps.status', {
        names: script.mayKeep.map((m) => m.name).join(', '),
      }),
      tone: 'neutral',
      testid: 'db-approval-kept-writes',
    });
  }
  out.push({
    text: t('studio.dbMcp.approval.risk.scriptResult.status'),
    tone: 'neutral',
    testid: 'db-approval-transform',
    tooltip: t('studio.dbMcp.approval.query.scriptResult.hint'),
  });
  return out;
}

// The tags for the kept variables a request uses; the testids carry the surface's prefix.
function keptRisks(kept: readonly KeptUse[], expiresAt: number, prefix: string): ApprovalRisk[] {
  if (kept.length === 0) return [];
  const out: ApprovalRisk[] = [
    {
      text: t('studio.kept.risk.uses.status', { names: kept.map((u) => u.name).join(', ') }),
      tone: 'neutral',
      testid: `${prefix}-kept`,
      tooltip: kept
        .map((u) => t('studio.kept.risk.source.label', { name: u.name, request: u.source.request }))
        .join('\n'),
    },
  ];
  if (expiresAt > 0 && kept.some((u) => Date.parse(u.expiresAt) < expiresAt)) {
    out.push({
      text: t('studio.kept.risk.expiring.status'),
      tone: 'warn',
      testid: `${prefix}-kept-expiring`,
    });
  }
  return out;
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
  if (p.inputs.some((i) => i.state === 'needed')) {
    out.push({
      text: t('studio.kept.risk.input.status'),
      tone: 'warn',
      testid: 'api-approval-input',
      tooltip: t('studio.kept.risk.input.tooltip'),
    });
  }
  out.push(
    ...keptRisks(
      p.kept.filter((u) => !u.pending),
      item.expiresAt,
      'api-approval',
    ),
  );
  if (p.script?.transform?.mayKeep.length) {
    out.push({
      text: t('studio.kept.risk.keeps.status', {
        names: p.script.transform.mayKeep.map((m) => m.name).join(', '),
      }),
      tone: 'neutral',
      testid: 'api-approval-kept-writes',
    });
  }
  if (p.script?.prepare) {
    out.push({
      text: t('studio.dbMcp.approval.risk.builtByScript.status'),
      tone: 'neutral',
      testid: 'api-approval-prepare',
      tooltip: t('studio.dbMcp.approval.api.scriptPrepare.hint'),
    });
  }
  if (p.script?.transform) {
    out.push({
      text: t('studio.dbMcp.approval.risk.scriptResult.status'),
      tone: 'neutral',
      testid: 'api-approval-transform',
      tooltip: t('studio.dbMcp.approval.api.scriptTransform.hint'),
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

function apiResponseRisks(item: ApprovalItem): ApprovalRisk[] {
  const p = apiResponsePayloadSchema.parse(item.payload);
  const out: ApprovalRisk[] = [];
  if (p.class === 'write') {
    out.push({
      text: t('studio.dbMcp.approval.risk.requestSent.status'),
      tone: 'warn',
      testid: 'api-review-sent',
    });
  }
  if (p.reason === 'userValue') {
    out.push({
      text: t('studio.kept.risk.userValue.status'),
      tone: 'warn',
      testid: 'api-review-user-value',
      tooltip: t('studio.kept.risk.userValue.tooltip'),
    });
  }
  if (p.isError) {
    out.push({
      text: t('studio.dbMcp.approval.risk.scriptFailed.status'),
      tone: 'warn',
      testid: 'api-review-script-failed',
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
      monaco: true,
      footerAction: markRaw(ReviewResultOption),
      operation: (item) => {
        const p = dbQueryPayloadSchema.parse(item.payload);
        if (p.explain) return t('studio.console.explain.label');
        return p.mode === 'masked'
          ? t('studio.dbMcp.approval.op.maskedRead.label')
          : dbOperation(item);
      },
      where: connectionWhere,
      risks: queryRisks,
    },
    'db.exemption': {
      content: markRaw(DbExemptionBody),
      monaco: true,
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
      monaco: true,
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
      operation: () => t('common.run.untitled.title'),
      where: (item) => item.target.location,
      whereData: true,
    },
    'api.request': {
      content: markRaw(ApiRequestBody),
      monaco: true,
      details: markRaw(ApiRequestDetails),
      footerAction: markRaw(ReviewResultOption),
      operation: (item) => apiRequestPayloadSchema.parse(item.payload).method,
      where: (item) =>
        apiRequestPayloadSchema.parse(item.payload).environment?.name ??
        t('studio.api.environments.none.option'),
      targetData: true,
      risks: apiRisks,
      requiredInputs: (item) =>
        apiRequestPayloadSchema
          .parse(item.payload)
          .inputs.filter((i) => i.state === 'needed')
          .map((i) => ({ name: i.name, label: i.label })),
      labels: () => ({
        approve: t('common.action.send.label'),
        deny: t('studio.dbMcp.approval.api.dontSend.label'),
      }),
    },
    'api.response': {
      content: markRaw(ApiResponseBody),
      monaco: true,
      details: markRaw(ApiResponseDetails),
      operation: () => t('common.field.result.label'),
      where: (item) =>
        apiResponsePayloadSchema.parse(item.payload).environment?.name ??
        t('studio.api.environments.none.option'),
      targetData: true,
      risks: apiResponseRisks,
      labels: () => ({
        approve: t('studio.dbMcp.preview.send.label'),
        deny: t('studio.dbMcp.preview.withhold.label'),
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
