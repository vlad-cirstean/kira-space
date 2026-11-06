import { compareNatural } from '@workbench/behaviour/match';
import type { Handover, TaskProof } from '../wire';
import type { TimelineEvent } from './taskTimeline';

export interface ChainSources {
  handovers: readonly Handover[];
  proofs: readonly TaskProof[];
  /** Empty while the Timeline toggle is off. */
  events: readonly TimelineEvent[];
}

// `layer` names the toggle that governs an entry: `chain` always shows, `timeline` only with the
// Timeline toggle on.
export type ChainEntry =
  | { kind: 'handover'; layer: 'chain'; key: string; at: number; handover: Handover }
  | { kind: 'proof'; layer: 'chain'; key: string; at: number; proof: TaskProof }
  | { kind: 'event'; layer: 'timeline'; key: string; at: number; event: TimelineEvent };

// Same millisecond: a proof with a diagram, a handover, any other proof, an event, task creation last.
function rank(e: ChainEntry): number {
  if (e.kind === 'handover') return 1;
  if (e.kind === 'event') return e.event.type === 'created' ? 4 : 3;
  return e.proof.steps.some((s) => s.view === 'diagram') ? 0 : 2;
}

/** Notes, handovers, attached proof and task events in one list, newest first. */
export function taskChain({ handovers, proofs, events }: ChainSources): ChainEntry[] {
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
  ];
  return entries.sort((a, b) => b.at - a.at || rank(a) - rank(b) || compareNatural(a.key, b.key));
}
