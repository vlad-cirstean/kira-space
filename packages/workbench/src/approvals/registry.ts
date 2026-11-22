import type { ApprovalItem } from '@shared/domain/approvals';
import { type Component, type InjectionKey, inject, provide } from 'vue';

export interface ApprovalRisk {
  text: string;
  tone: 'warn' | 'neutral' | 'error';
  testid?: string;
  /** Full detail behind the short tag. */
  tooltip?: string;
  /** Extra attributes on the tag (a `data-*` a spec reads). */
  attrs?: Record<string, string>;
}

/** The verdict badge row: what the request does, and the facts that bound it. */
export interface ApprovalVerdict {
  label: string;
  tone: 'ok' | 'warn' | 'error' | 'neutral';
  /** The kind's own hue, one per kind, so Explain, Read, Write, Schema change and Delete differ. */
  hue?: 'explain' | 'read' | 'write' | 'schema' | 'delete';
  icon: string;
  /** A small lock inside the kind badge: the action is read-only (wording in `tooltip`). */
  lock?: boolean;
  tooltip?: string;
  facts?: {
    text: string;
    tone: 'ok' | 'warn' | 'neutral' | 'caution';
    icon: string;
    tooltip?: string;
    /** Replaces the default `approval-fact` testid. */
    testid?: string;
  }[];
}

/** One piece of the target row's "where": drawn side by side with a gap, never joined by a separator. */
export interface ApprovalWherePart {
  text: string;
  /** Names the part (`data-part` on it): host, database, namespace, environment. */
  label?: string;
  /** Drawn as a chip. */
  chip?: boolean;
  /** Data font (host, path, folder). */
  data?: boolean;
  /** A connection colour: a dot before a chip's text. */
  color?: string;
}

/**
 * What an app supplies for one approval kind. The card draws the target row from the target and
 * `where`, the badge row from `verdict` (`operation` when absent), `content` in a framed box (or its own boxes with `sections`),
 * `risks` as a warnings list under it, and `details` (label/value rows) behind Details.
 * `footerAction` is a control beside Deny and Approve. Every component takes `{ item }`;
 * `content` and `footerAction` may emit `options`.
 */
export interface ApprovalKindDef {
  content: Component;
  /** The operation word: the badge when `verdict` is absent. */
  operation: (item: ApprovalItem) => string;
  /** The badge row: kind badge and facts. */
  verdict?: (item: ApprovalItem) => ApprovalVerdict;
  /** A mark beside the target name (the engine icon); takes `{ item }`. */
  targetIcon?: Component;
  where?: (item: ApprovalItem) => ApprovalWherePart[];
  risks?: (item: ApprovalItem) => ApprovalRisk[];
  /** The "Asked by" row of Details; the origin (client, folder, task, step, source) when absent. */
  requester?: (item: ApprovalItem) => string;
  details?: Component;
  /** `content` draws its own `ApprovalBox` per section; the card adds no frame. */
  sections?: boolean;
  footerAction?: Component;
  /** The target is data (host, path): data font. */
  targetData?: boolean;
  /** The body renders in Monaco: the surface preloads it as the dialog opens, so the editor does not
   *  pop in. Absent for a kind with no editor, which then never pays for Monaco. */
  monaco?: boolean;
  /** Values the person must type before Approve enables (declared by the agent, kept as secrets). */
  requiredInputs?: (item: ApprovalItem) => { name: string; label: string }[];
  /** Button words when Approve/Deny do not fit (Send/Withhold, Accept/Reject). */
  labels?: (item: ApprovalItem) => { approve: string; deny: string };
}

export type ApprovalKinds = Record<string, ApprovalKindDef>;

const approvalKindsKey: InjectionKey<ApprovalKinds> = Symbol('approvalKinds');

export function provideApprovalKinds(kinds: ApprovalKinds): void {
  provide(approvalKindsKey, kinds);
}

export function useApprovalKinds(): ApprovalKinds {
  return inject(approvalKindsKey, {});
}

export function useApprovalKind(kind: string): ApprovalKindDef | undefined {
  return useApprovalKinds()[kind];
}
