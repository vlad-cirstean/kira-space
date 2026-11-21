import type { ApprovalItem } from '@shared/domain/approvals';
import type { ConnectionKind } from '@shared/domain/connection';
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
  kubeResponsePayloadSchema,
} from '@shared/domain/dbmcp';
import type { KeptUse } from '@shared/domain/kept';
import { proofReviewPayloadSchema } from '@shared/domain/proof';
import { joinSentences, type MessageKey, t } from '@theme/i18n';
import type { ApprovalKinds, ApprovalRisk, ApprovalVerdict } from '@workbench/approvals/registry';
import { formatCount, formatDuration } from '@workbench/behaviour/format';
import { type FunctionalComponent, h, markRaw } from 'vue';
import ClusterIcon from '../kubernetes/components/ClusterIcon.vue';
import EngineIcon from '../theme/EngineIcon.vue';
import ApiRequestBody from './ApiRequestBody.vue';
import ApiRequestDetails from './ApiRequestDetails.vue';
import ApiResponseBody from './ApiResponseBody.vue';
import ApiResponseDetails from './ApiResponseDetails.vue';
import DbExemptionBody from './DbExemptionBody.vue';
import DbExemptionDetails from './DbExemptionDetails.vue';
import DbQueryBody from './DbQueryBody.vue';
import DbQueryDetails from './DbQueryDetails.vue';
import DbResponseBody from './DbResponseBody.vue';
import DbResponseDetails from './DbResponseDetails.vue';
import KubeReadBody from './KubeReadBody.vue';
import KubeReadDetails from './KubeReadDetails.vue';
import KubeResponseBody from './KubeResponseBody.vue';
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

type Fact = NonNullable<ApprovalVerdict['facts']>[number];

const READ_ONLY_TOOLTIP: Record<string, MessageKey> = {
  session: 'studio.dbMcp.approval.readOnly.session.hint',
  app: 'studio.dbMcp.approval.readOnly.app.hint',
  none: 'studio.dbMcp.approval.readOnly.none.hint',
};

// Every read runs on the read lane, and a read-only connection never reaches a write prompt (the
// permission check refuses write and ddl first), so a read prompt is read-only by enforcement.
function readOnlyWording(enforcement: string, connReadOnly: boolean): string {
  return joinSentences(
    t(READ_ONLY_TOOLTIP[enforcement] ?? READ_ONLY_TOOLTIP.none),
    connReadOnly && t('studio.dbMcp.approval.readOnly.connection.hint'),
  );
}

function dbVerdict(item: ApprovalItem): ApprovalVerdict {
  const p = (item.kind === 'db.exemption' ? dbExemptionPayloadSchema : dbQueryPayloadSchema).parse(
    item.payload,
  );
  const facts: Fact[] = [];
  if ('script' in p && p.script) {
    facts.push({
      text: t('studio.dbMcp.approval.fact.scripts.status', { count: 1 }),
      tone: 'warn',
      icon: 'code',
      tooltip: t('studio.dbMcp.approval.query.scriptResult.hint'),
    });
  }
  const cls = item.operation.class;
  if (cls === 'read') {
    const wording = readOnlyWording(p.enforcement, item.target.readOnly);
    // Kira's own check is not the database's, so it earns a badge and the kind badge carries no lock.
    const lock = p.enforcement === 'app' ? {} : { lock: true, tooltip: wording };
    if (p.enforcement === 'app') {
      facts.unshift({
        text: t('studio.dbMcp.approval.fact.checkedByKira.status'),
        tone: 'caution',
        icon: 'shield',
        tooltip: wording,
      });
    }
    if ('explain' in p && p.explain) {
      return {
        label: t('studio.console.explain.label'),
        tone: 'ok',
        hue: 'explain',
        icon: 'graph',
        facts,
        ...lock,
      };
    }
    const label =
      p.mode === 'masked'
        ? t('studio.dbMcp.approval.op.maskedRead.label')
        : t('studio.project.connectionDialog.mcp.read.label');
    return { label, tone: 'ok', hue: 'read', icon: 'eye', facts, ...lock };
  }
  if (cls === 'write') {
    return {
      label: t('studio.project.connectionDialog.mcp.write.label'),
      tone: 'warn',
      hue: 'write',
      icon: 'edit',
      facts,
    };
  }
  if (cls === 'ddl') {
    return {
      label: t('studio.dbMcp.approval.verdict.schemaChange.label'),
      tone: 'error',
      hue: 'schema',
      icon: 'symbol-structure',
      facts,
    };
  }
  return {
    label: t('studio.dbMcp.approval.target.unclassified.label'),
    tone: 'warn',
    icon: 'question',
    facts,
  };
}

// "Lift limit" does not say read, so a read-only exemption keeps a separate green Read-only badge.
function exemptionVerdict(item: ApprovalItem): ApprovalVerdict {
  const v = dbVerdict(item);
  const facts =
    v.lock && v.tooltip
      ? [
          {
            text: t('common.state.readOnly.status'),
            tone: 'ok' as const,
            icon: 'lock',
            tooltip: v.tooltip,
          },
          ...(v.facts ?? []),
        ]
      : v.facts;
  return {
    ...v,
    label: t('studio.dbMcp.approval.op.liftLimit.label'),
    tone: 'warn',
    hue: 'write',
    icon: 'unlock',
    lock: false,
    tooltip: undefined,
    facts,
  };
}

// The held result: one badge naming what produced it. The table and the Send / Withhold buttons
// already say the rest.
function resultVerdict(read: boolean, ranTooltip: MessageKey): ApprovalVerdict {
  return read
    ? {
        label: t('studio.dbMcp.approval.verdict.readResult.label'),
        tone: 'ok',
        hue: 'read',
        icon: 'eye',
      }
    : {
        label: t('studio.dbMcp.approval.verdict.writeResult.label'),
        tone: 'warn',
        hue: 'write',
        icon: 'edit',
        tooltip: t(ranTooltip),
      };
}

const DbTargetIcon: FunctionalComponent<{ item: ApprovalItem }> = (p) =>
  h(EngineIcon, { kind: p.item.target.kind as ConnectionKind, size: 16 });

function apiVerdict(item: ApprovalItem): ApprovalVerdict {
  const p = apiRequestPayloadSchema.parse(item.payload);
  const facts: Fact[] = [];
  const scripts = [
    p.script?.prepare && t('studio.dbMcp.approval.api.scriptPrepare.hint'),
    p.script?.transform && t('studio.dbMcp.approval.api.scriptTransform.hint'),
  ].filter((x): x is string => !!x);
  if (scripts.length > 0) {
    facts.push({
      text: t('studio.dbMcp.approval.fact.scripts.status', { count: scripts.length }),
      tone: 'warn',
      icon: 'code',
      tooltip: scripts.join('\n'),
    });
  }
  // One badge names everything the request sends that the user did not write: shared secrets, then the values
  // typed on the card (once or kept). The card's rows carry no second list.
  const sent = [...new Set([...p.credentials.map((c) => c.name), ...p.inputs.map((i) => i.name)])];
  if (sent.length > 0) {
    facts.push({
      text: t('studio.dbMcp.approval.api.sends.status', { names: sent.join(', ') }),
      tone: isRead(p.method) ? 'neutral' : 'warn',
      icon: 'key',
      tooltip: [
        ...p.credentials.map((c) =>
          t('studio.dbMcp.approval.api.credential.label', {
            name: c.name,
            hosts: c.hosts.join(', '),
          }),
        ),
        ...p.inputs.map((i) =>
          t('studio.dbMcp.approval.api.input.label', {
            name: i.name,
            host: i.origin.replace(/^https?:\/\//, ''),
          }),
        ),
      ].join('\n'),
    });
  }
  if (isRead(p.method)) {
    return {
      label: t('studio.project.connectionDialog.mcp.read.label'),
      tone: 'ok',
      hue: 'read',
      icon: 'eye',
      facts,
    };
  }
  if (p.method === 'DELETE') {
    return {
      label: t('common.action.delete.label'),
      tone: 'error',
      hue: 'delete',
      icon: 'trash',
      facts,
    };
  }
  return {
    label: t('studio.project.connectionDialog.mcp.write.label'),
    tone: 'warn',
    hue: 'write',
    icon: 'edit',
    facts,
  };
}
const MAX_PLAN_ISSUES = 2;

function dbOperation(item: ApprovalItem): string {
  const key = DB_CLASS_LABEL[item.operation.class];
  return key ? t(key) : item.operation.class;
}

// "PostgreSQL · db.internal · shop / public": the connection's kind label, then its location.
function connectionWhere(item: ApprovalItem): string {
  const kind = connectionKindSchema.safeParse(item.target.kind).data;
  // The engine icon names a known kind; text only for an unknown one.
  return [kind ? '' : item.target.kind, item.target.location].filter(Boolean).join(' · ');
}

// The plan's estimate over its threshold, then its issues: two tags, the rest as "+N more".
function planRisks(plan: DbMcpApprovalPlan | null, withRows: boolean): ApprovalRisk[] {
  if (!plan) return [];
  const out: ApprovalRisk[] = [];
  if (withRows && plan.estimatedRowsRead !== null && plan.estimatedRowsRead > plan.thresholdRows) {
    out.push({
      text: t('studio.dbMcp.approval.plan.rowsOver.status', {
        rows: formatCount(plan.estimatedRowsRead),
        limit: formatCount(plan.thresholdRows),
      }),
      tone: 'warn',
      testid: 'db-mcp-approval-plan-rows',
    });
  } else if (
    withRows &&
    plan.estimatedRowsRead !== null &&
    plan.issues.some((i) => i.severity === 'warn')
  ) {
    // The estimate frames the warnings; a clean plan shows nothing.
    out.push({
      text: t('studio.dbMcp.approval.plan.rowsUnder.status', {
        rows: formatCount(plan.estimatedRowsRead),
        limit: formatCount(plan.thresholdRows),
      }),
      tone: 'neutral',
      testid: 'db-mcp-approval-plan-rows',
    });
  }
  for (const issue of plan.issues.slice(0, MAX_PLAN_ISSUES)) {
    out.push({
      text: issue.message,
      tone: issue.severity === 'warn' ? 'warn' : 'neutral',
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
        : t('studio.dbMcp.approval.exemption.limitHit.status', {
            measured: show(l.measured),
            max: show(l.max),
          });
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
    {
      text: t('studio.dbMcp.approval.exemption.reason.status', { reason: p.reason }),
      tone: 'neutral',
      testid: 'db-mcp-exemption-reason',
    },
    ...out,
    ...planRisks(p.plan, !hasRowsLimit),
    ...keptRisks(p.kept, item.expiresAt, 'db-approval'),
    ...queryScriptRisks(p.script),
  ];
}

// A name the script may keep, marked when a kept.setSecret call writes it.
const mayKeepName = (m: { name: string; secret: boolean }): string =>
  m.secret ? t('studio.kept.risk.secretName.label', { name: m.name }) : m.name;

// The tag naming what a transform script may keep; none when it keeps nothing.
function mayKeepRisks(
  script: { mayKeep: readonly { name: string; secret: boolean }[] } | null | undefined,
  testid: string,
): ApprovalRisk[] {
  if (!script?.mayKeep.length) return [];
  return [
    {
      text: t('studio.kept.risk.keeps.status', {
        names: script.mayKeep.map(mayKeepName).join(', '),
      }),
      tone: 'neutral',
      testid,
    },
  ];
}

// The tags for a run_query result script: what it may keep and that the agent sees its output, not the rows.
function queryScriptRisks(script: DbQueryPayload['script']): ApprovalRisk[] {
  return mayKeepRisks(script, 'db-approval-kept-writes');
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
  out.push(
    ...keptRisks(
      p.kept.filter((u) => !u.pending),
      item.expiresAt,
      'api-approval',
    ),
  );
  out.push(...mayKeepRisks(p.script?.transform, 'api-approval-kept-writes'));
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

const KubeTargetIcon: FunctionalComponent = () => h(ClusterIcon, { size: 16 });

// Every cluster read is a GET: one Read badge, a lock for the guarantee, and a fact for the scripts.
function kubeVerdict(item: ApprovalItem): ApprovalVerdict {
  const p = kubeReadPayloadSchema.parse(item.payload);
  const scripts = [
    p.script?.prepare && t('studio.dbMcp.approval.kube.scriptPrepare.hint'),
    p.script?.transform && t('studio.dbMcp.approval.kube.scriptTransform.hint'),
  ].filter((x): x is string => !!x);
  return {
    label: t('studio.project.connectionDialog.mcp.read.label'),
    tone: 'ok',
    hue: 'read',
    icon: 'eye',
    lock: true,
    tooltip: t('studio.dbMcp.approval.kube.readOnly.hint'),
    facts: scripts.length
      ? [
          {
            text: t('studio.dbMcp.approval.fact.scripts.status', { count: scripts.length }),
            tone: 'warn',
            icon: 'code',
            tooltip: scripts.join('\n'),
          },
        ]
      : [],
  };
}

function kubeRisks(item: ApprovalItem): ApprovalRisk[] {
  const p = kubeReadPayloadSchema.parse(item.payload);
  const out = [
    ...keptRisks(p.kept, item.expiresAt, 'kube-approval'),
    ...mayKeepRisks(p.script?.transform, 'kube-approval-kept-writes'),
  ];
  if (p.operation === 'logs') {
    out.push({
      text: t('studio.dbMcp.approval.kube.logsUnmasked.status'),
      tone: 'neutral',
      testid: 'kube-approval-logs-unmasked',
    });
  }
  if (p.operation === 'describe' && p.request?.kind === 'v1/secrets') {
    out.push({
      text: t('studio.dbMcp.approval.kube.secretBytes.status'),
      tone: 'neutral',
      testid: 'kube-approval-secret-bytes',
    });
  }
  return out;
}

// The location already ends in the namespace when one is set.
function kubeWhere(
  schema: typeof kubeReadPayloadSchema | typeof kubeResponsePayloadSchema,
): (item: ApprovalItem) => string {
  return (item) =>
    [
      item.target.location,
      schema.parse(item.payload).namespace ? '' : t('kubernetes.namespace.all.option'),
    ]
      .filter(Boolean)
      .join(' · ');
}

// P520: Studio's approval kinds (internal/approvals Kind* constants). Each maps a subject to its
// renderers; the shared surface supplies the chrome.
export function studioApprovalKinds(): ApprovalKinds {
  return {
    'db.query': {
      content: markRaw(DbQueryBody),
      details: markRaw(DbQueryDetails),
      monaco: true,
      footerAction: markRaw(ReviewResultOption),
      verdict: dbVerdict,
      targetIcon: markRaw(DbTargetIcon),
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
      verdict: exemptionVerdict,
      targetIcon: markRaw(DbTargetIcon),
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
      verdict: (item) => {
        const p = dbResponsePayloadSchema.parse(item.payload);
        return resultVerdict(
          !p.alreadyRan && (p.mode === undefined || p.mode === 'read' || p.mode === 'masked'),
          'studio.dbMcp.approval.verdict.dbWriteResult.hint',
        );
      },
      targetIcon: markRaw(DbTargetIcon),
      where: connectionWhere,
      labels: () => ({
        approve: t('studio.dbMcp.preview.send.label'),
        deny: t('studio.dbMcp.preview.withhold.label'),
      }),
    },
    'kube.read': {
      content: markRaw(KubeReadBody),
      details: markRaw(KubeReadDetails),
      monaco: true,
      targetIcon: markRaw(KubeTargetIcon),
      verdict: kubeVerdict,
      risks: kubeRisks,
      footerAction: markRaw(ReviewResultOption),
      operation: (item) =>
        t(KUBE_OPERATION_LABEL[kubeReadPayloadSchema.parse(item.payload).operation]),
      // The location already ends in the namespace when one is set.
      where: kubeWhere(kubeReadPayloadSchema),
    },
    'kube.response': {
      content: markRaw(KubeResponseBody),
      monaco: true,
      operation: () => t('common.field.result.label'),
      verdict: () => resultVerdict(true, 'studio.dbMcp.approval.verdict.dbWriteResult.hint'),
      where: kubeWhere(kubeResponsePayloadSchema),
      labels: () => ({
        approve: t('studio.dbMcp.preview.send.label'),
        deny: t('studio.dbMcp.preview.withhold.label'),
      }),
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
      verdict: apiVerdict,
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
      verdict: (item) =>
        resultVerdict(
          apiResponsePayloadSchema.parse(item.payload).class === 'read',
          'studio.dbMcp.approval.verdict.apiWriteResult.hint',
        ),
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
