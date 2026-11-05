import { trackingDefaults } from '../../../state/trackingConcepts';
import type { TrackedUpdate } from '../wire';

export type TrackingSettings = Record<string, Record<string, boolean>>;

/** The updates left once the tracking settings drop concepts the user turned off. */
export function meaningfulUpdates(
  updates: readonly TrackedUpdate[],
  tracking: TrackingSettings,
): TrackedUpdate[] {
  const out: TrackedUpdate[] = [];
  for (const u of updates) {
    const defaults = trackingDefaults(u.kind);
    const concepts = u.concepts.filter((c) => (tracking[u.kind]?.[c] ?? defaults[c]) === true);
    if (concepts.length > 0) out.push({ ...u, concepts });
  }
  return out;
}
