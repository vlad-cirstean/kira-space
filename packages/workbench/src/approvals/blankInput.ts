import type { ApprovalItem } from '@shared/domain/approvals';
import { logWarn } from '../bridge/log';
import type { ApprovalKindDef } from './registry';

/** The label of the first required input still blank after trim, or null when Approve may go. */
export function blankInput(
  def: ApprovalKindDef | undefined,
  item: ApprovalItem,
  typed: Record<string, string> | undefined,
): string | null {
  try {
    const missing = def?.requiredInputs?.(item).find((i) => !typed?.[i.name]?.trim());
    return missing ? missing.label : null;
  } catch (err) {
    logWarn('approvals', `kind ${item.kind} inputs unreadable`, err);
    return null;
  }
}
