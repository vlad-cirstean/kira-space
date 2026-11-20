import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';
import { routed } from '@workbench/testing/ui/prompts';
import { FIXED_NOW } from './adeV2';
import { contract } from './contract';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

/** Lists answers that already hold `items`, so the first fetch cannot race a later push. */
export function seedApprovals(items: ApprovalItem[]): ControlSnapshot[] {
  return [
    { channel: IPC.approvalsList, response: items },
    { channel: IPC.promptsList, response: items.map((i) => routed('approval', i.id)) },
  ];
}

/** The live details.edit approval a contract scenario recorded. */
export function detailsEditItem(scenario: string, key = 'detailsEdit'): ApprovalItem {
  const [it] = contract<ApprovalItem[]>(scenario, `ApprovalsService.List#${key}`);
  if (!it) throw new Error('contract has no details.edit item');
  return liveItem(it, 1800, FIXED_NOW);
}
