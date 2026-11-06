import { compareNatural } from '@workbench/behaviour/match';
import type { Handover, TaskProof } from '../wire';

export type ChainEntry =
  | { kind: 'handover'; key: string; at: number; handover: Handover }
  | { kind: 'proof'; key: string; at: number; proof: TaskProof };

// Same millisecond: a proof with a diagram, then a handover, then any other proof.
function rank(e: ChainEntry): number {
  if (e.kind === 'handover') return 1;
  return e.proof.steps.some((s) => s.view === 'diagram') ? 0 : 2;
}

/** Notes, handovers and attached proof in one list, newest first. */
export function taskChain(
  handovers: readonly Handover[],
  proofs: readonly TaskProof[],
): ChainEntry[] {
  const entries: ChainEntry[] = [
    ...handovers.map(
      (handover): ChainEntry => ({
        kind: 'handover',
        key: `handover:${handover.id}`,
        at: handover.state === 'draft' ? handover.updatedAt : handover.createdAt,
        handover,
      }),
    ),
    ...proofs.map(
      (proof): ChainEntry => ({
        kind: 'proof',
        key: `proof:${proof.proofId}`,
        at: proof.closedAt,
        proof,
      }),
    ),
  ];
  return entries.sort((a, b) => b.at - a.at || rank(a) - rank(b) || compareNatural(a.key, b.key));
}
