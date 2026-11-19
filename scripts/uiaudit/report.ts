export interface Site {
  file: string;
  line: number;
  module: string;
  note?: string;
}

export interface Cluster {
  /** Stable group key: sha1 of the normalised signature, 10 hex. */
  key: string;
  title: string;
  /** Channel-defined magnitude (element count, token count, site count); see `sizeUnit`. */
  size: number;
  sizeUnit: string;
  sites: Site[];
  note?: string;
}

export interface ChannelResult {
  id: string;
  title: string;
  clusters: Cluster[];
}

function moduleSet(c: Cluster): string[] {
  return [...new Set(c.sites.map((s) => s.module))].sort();
}

/** Module spread first, then site count, then key for a stable order. */
export function sortClusters(clusters: Cluster[]): Cluster[] {
  return [...clusters].sort(
    (a, b) =>
      moduleSet(b).length - moduleSet(a).length ||
      b.sites.length - a.sites.length ||
      (a.key < b.key ? -1 : 1),
  );
}

function renderChannel(r: ChannelResult): string {
  const out = [`## ${r.id.toUpperCase()} ${r.title}`, '', `${r.clusters.length} clusters.`, ''];
  for (const c of r.clusters) {
    const mods = moduleSet(c);
    out.push(
      `### \`${c.key}\` ${c.title}`,
      '',
      `size ${c.size} ${c.sizeUnit}, ${c.sites.length} sites, ${mods.length} modules: ${mods.join(', ')}`,
    );
    if (c.note) out.push('', c.note);
    out.push('');
    const seen = new Set<string>();
    for (const s of [...c.sites].sort((a, b) =>
      a.file < b.file ? -1 : a.file > b.file ? 1 : a.line - b.line,
    )) {
      const line = `- ${s.file}:${s.line}${s.note ? ` ${s.note}` : ''}`;
      if (seen.has(line)) continue;
      seen.add(line);
      out.push(line);
    }
    out.push('');
  }
  return out.join('\n');
}

export function renderReport(
  results: ChannelResult[],
  meta: { files: number; vue: number; ts: number },
): string {
  const head = [
    '# UI audit report',
    '',
    `Scan set: ${meta.files} files (${meta.vue} vue, ${meta.ts} ts). Heuristic and noisy by design; triage decides.`,
    '',
    '| Channel | Clusters |',
    '|---|---|',
    ...results.map((r) => `| ${r.id.toUpperCase()} ${r.title} | ${r.clusters.length} |`),
    '',
  ];
  return `${head.join('\n')}\n${results.map(renderChannel).join('\n')}`;
}
