import type { ApprovalItem } from '@shared/domain/approvals';
import { apiRequestPayloadSchema, kubeReadPayloadSchema } from '@shared/domain/dbmcp';
import { type MessageKey, t } from '@theme/i18n';
import type { ApprovalKinds } from '@workbench/approvals/registry';
import { markRaw } from 'vue';
import ApiRequestBody from './ApiRequestBody.vue';
import ConnectionHeader from './ConnectionHeader.vue';
import DbQueryBody from './DbQueryBody.vue';
import DbResponseBody from './DbResponseBody.vue';
import KubeReadBody from './KubeReadBody.vue';
import ProofReviewBody from './ProofReviewBody.vue';
import ScriptRunBody from './ScriptRunBody.vue';

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

function dbOperation(item: ApprovalItem): string {
  const key = DB_CLASS_LABEL[item.operation.class];
  return key ? t(key) : item.operation.class;
}

// P520: Studio's approval kinds (internal/approvals Kind* constants). Each maps a subject to its
// renderers; the shared surface supplies the chrome.
export function studioApprovalKinds(): ApprovalKinds {
  return {
    'db.query': {
      header: markRaw(ConnectionHeader),
      body: markRaw(DbQueryBody),
      operation: dbOperation,
    },
    'db.response': {
      header: markRaw(ConnectionHeader),
      body: markRaw(DbResponseBody),
      operation: dbOperation,
      labels: () => ({
        approve: t('studio.dbMcp.preview.send.label'),
        deny: t('studio.dbMcp.preview.withhold.label'),
      }),
    },
    'kube.read': {
      header: markRaw(ConnectionHeader),
      body: markRaw(KubeReadBody),
      operation: (item) =>
        t(KUBE_OPERATION_LABEL[kubeReadPayloadSchema.parse(item.payload).operation]),
    },
    'script.run': {
      header: markRaw(ConnectionHeader),
      body: markRaw(ScriptRunBody),
      operation: () => t('studio.dbMcp.approval.script.operation.label'),
    },
    'api.request': {
      header: markRaw(ConnectionHeader),
      body: markRaw(ApiRequestBody),
      operation: (item) => apiRequestPayloadSchema.parse(item.payload).method,
      labels: () => ({
        approve: t('common.action.send.label'),
        deny: t('studio.dbMcp.approval.api.dontSend.label'),
      }),
    },
    'proof.review': {
      body: markRaw(ProofReviewBody),
      labels: () => ({
        approve: t('studio.proof.change.accept.label'),
        deny: t('studio.proof.change.reject.label'),
      }),
    },
  };
}
