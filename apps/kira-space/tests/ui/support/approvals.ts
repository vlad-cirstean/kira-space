import type { ApprovalItem } from '@shared/domain/approvals';
import { routed } from '@workbench/testing/ui/prompts';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

/** Lists answers that already hold `items`, so the first fetch cannot race a later push. */
export function seedApprovals(items: ApprovalItem[]): ControlSnapshot[] {
  return [
    { channel: IPC.approvalsList, response: items },
    { channel: IPC.promptsList, response: items.map((i) => routed('approval', i.id)) },
  ];
}
