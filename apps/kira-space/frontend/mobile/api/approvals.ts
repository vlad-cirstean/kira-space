import type { ApprovalAnswerArgs, ApprovalItem, ApprovalResult } from '@shared/domain/approvals';
import { newUuid } from '@shared/ids';
import { CHANNEL } from '@shared/protocol/events';
import type { ApprovalsControl } from '@workbench/approvals/approvalsQueries';
import { onChannel } from './events';
import { getJson, postJson } from './http';

// The phone's approvals queue: Space kinds only, paths stripped by the server (mobileweb/approvals.go).
export const httpApprovalsControl: ApprovalsControl = {
  approvalsList: () => getJson<ApprovalItem[]>('/api/approvals'),
  // One key per click: one intent, and useApprovalActions never retries.
  approvalsAnswer: (args: ApprovalAnswerArgs) =>
    postJson<ApprovalResult[]>('/api/approvals/answer', args, { idempotencyKey: newUuid() }),
  onApprovalsChanged: (cb) => onChannel(CHANNEL.approvalsChanged, (d) => cb(d as ApprovalItem[])),
};
