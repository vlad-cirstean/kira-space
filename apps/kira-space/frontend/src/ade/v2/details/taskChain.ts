import { compareNatural } from '@workbench/behaviour/match';
import type { DbAuditEntry, Handover, TaskProof } from '../wire';
import type { TimelineEvent } from './taskTimeline';

export interface ChainSources {
  handovers: readonly Handover[];
  proofs: readonly TaskProof[];
  /** Empty while the Timeline toggle is off. */
  events: readonly TimelineEvent[];
  /** Empty while the Kira DB toggle is off. */
  audit: readonly DbAuditEntry[];
}

// `layer` names the toggle that governs an entry: `chain` always shows, `timeline` only with the
// Timeline toggle on, `audit` only with the Kira DB toggle on.
export type ChainEntry =
  | { kind: 'handover'; layer: 'chain'; key: string; at: number; handover: Handover }
  | { kind: 'proof'; layer: 'chain'; key: string; at: number; proof: TaskProof }
  | { kind: 'event'; layer: 'timeline'; key: string; at: number; event: TimelineEvent }
  | { kind: 'audit'; layer: 'audit'; key: string; at: number; audit: DbAuditEntry };

// Same millisecond: a proof with a diagram, a handover, any other proof, a Kira DB action, an event,
// task creation last.
function rank(e: ChainEntry): number {
  if (e.kind === 'handover') return 1;
  if (e.kind === 'audit') return 3;
  if (e.kind === 'event') return e.event.type === 'created' ? 5 : 4;
  return e.proof.steps.some((s) => s.view === 'diagram') ? 0 : 2;
}

/** Notes, handovers, attached proof, Kira DB actions and task events in one list, newest first. */
export function taskChain({ handovers, proofs, events, audit }: ChainSources): ChainEntry[] {
  const entries: ChainEntry[] = [
    ...handovers.map(
      (handover): ChainEntry => ({
        kind: 'handover',
        layer: 'chain',
        key: `handover:${handover.id}`,
        at: handover.state === 'draft' ? handover.updatedAt : handover.createdAt,
        handover,
      }),
    ),
    ...proofs.map(
      (proof): ChainEntry => ({
        kind: 'proof',
        layer: 'chain',
        key: `proof:${proof.proofId}`,
        at: proof.closedAt,
        proof,
      }),
    ),
    ...events.map(
      (event): ChainEntry => ({
        kind: 'event',
        layer: 'timeline',
        key: event.key,
        at: event.at,
        event,
      }),
    ),
    ...audit.map(
      (entry): ChainEntry => ({
        kind: 'audit',
        layer: 'audit',
        key: `audit:${entry.id}`,
        at: entry.startedAt,
        audit: entry,
      }),
    ),
  ];
  return entries.sort((a, b) => b.at - a.at || rank(a) - rank(b) || compareNatural(a.key, b.key));
}
