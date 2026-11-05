import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { CHANNEL } from '@shared/protocol/events';
import { emitWailsEvent } from './mockRuntime';
import { emitPrompts, routed } from './prompts';

/** A recorded item given a live clock: it expires `seconds` from now. */
export function liveItem(item: ApprovalItem, seconds = 600, createdAt = Date.now()): ApprovalItem {
  return { ...item, createdAt, expiresAt: createdAt + seconds * 1000 };
}

/** Pushes the queue's whole list and the router entries that target the UI suite's window. */
export async function pushApprovals(page: Page, items: ApprovalItem[]): Promise<void> {
  await emitWailsEvent(page, CHANNEL.approvalsChanged, items);
  await emitPrompts(
    page,
    items.map((i) => routed('approval', i.id)),
  );
}

/** The mock answer for an ApprovalsService.Answer call: every id accepted. */
export function answeredOk(ids: string[]): { id: string; code: string; error: string }[] {
  return ids.map((id) => ({ id, code: '', error: '' }));
}
