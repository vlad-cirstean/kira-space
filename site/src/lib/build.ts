import { BUILD_SECTIONS, type BuildSection } from '../data/build';
import { SHOT_CAPTURE } from '../data/shotCapture';
import { type ResolvedShot, type ResolvedWindowShot, resolveShot } from './shots';

export interface ResolvedBuildSection extends BuildSection {
  readonly resolved: readonly ResolvedShot[];
  readonly lead: ResolvedWindowShot;
}

const isBuildId = (id: string) => id.startsWith('build-');

/** Resolves the Build page sections; throws at build time when the shot lists break a rule. */
export async function resolveBuild(): Promise<ResolvedBuildSection[]> {
  const seen = new Map<string, string>();
  let total = 0;
  for (const section of BUILD_SECTIONS) {
    if (section.shots.length < 4 || section.shots.length > 6)
      throw new Error(
        `Build section ${section.anchor} has ${section.shots.length} shots, not 4 to 6`,
      );
    for (const id of section.shots) {
      const other = seen.get(id);
      if (other) throw new Error(`Build shot ${id} is in ${other} and ${section.anchor}`);
      seen.set(id, section.anchor);
      total += 1;
    }
    const first = section.shots[0];
    if (!first || SHOT_CAPTURE[first].kind !== 'window')
      throw new Error(`Build section ${section.anchor} must lead with a window shot`);
  }
  if (total < 20 || total > 30) throw new Error(`Build page has ${total} shots, not 20 to 30`);
  for (const id of Object.keys(SHOT_CAPTURE)) {
    if (isBuildId(id) && !seen.has(id)) throw new Error(`Shot ${id} is in no Build section`);
  }
  return Promise.all(
    BUILD_SECTIONS.map(async (section) => {
      const resolved = await Promise.all(section.shots.map((id) => resolveShot(id)));
      const lead = resolved[0];
      if (lead?.kind !== 'window') throw new Error(`Build ${section.anchor} lead`);
      return { ...section, resolved, lead };
    }),
  );
}
