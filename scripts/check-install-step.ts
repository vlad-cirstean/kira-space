// Replays internal/releaseline/testdata/next_step_index.json (the Go NextStep table, rendered as
// release indexes by FormatIndex) through scripts/install.sh's awk index_next_step, and checks the
// JXA apiToIndex fallback builds the same index text from the API-shaped releases
// (verify-packaging S16).
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

interface Golden {
  name: string;
  current: string;
  index: string;
  target: string;
}
interface Case {
  name: string;
  releases: { tag_name: string }[] | null;
}

const golden = JSON.parse(
  readFileSync('internal/releaseline/testdata/next_step_index.json', 'utf8'),
) as Golden[];
const dir = mkdtempSync(join(tmpdir(), 'check-install-step-'));
let bad = 0;
let checked = 0;
try {
  for (const g of golden) {
    // An empty index is a case FormatIndex rejects; install.sh never sees one.
    if (g.index === '') continue;
    const file = join(dir, 'index.txt');
    writeFileSync(file, g.index);
    const r = spawnSync(
      'sh',
      [
        '-c',
        '. scripts/install.sh; ASSET=app.dmg; index_check "$1" && index_next_step "$1" "$2"',
        'sh',
        file,
        g.current,
      ],
      { encoding: 'utf8', env: { ...process.env, KIRA_INSTALL_SOURCE_ONLY: '1' } },
    );
    const got = r.stdout.trim();
    const want = g.target === '' ? '' : `v${g.target}`;
    if (r.status !== 0 || got !== want) {
      console.error(
        `check-install-step: "${g.name}": install.sh picks '${got}' (exit ${r.status}) ${r.stderr}, Go table wants '${want}'`,
      );
      bad++;
    }
    checked++;
  }
} finally {
  rmSync(dir, { recursive: true, force: true });
}

// Fallback parity: apiToIndex must render the same text as releaseline.FormatIndex.
const script = readFileSync('scripts/install.sh', 'utf8');
const m = /<<'API_INDEX_JXA_EOF'\n([\s\S]*?)\nAPI_INDEX_JXA_EOF\n/.exec(script);
if (!m?.[1]) {
  console.error('check-install-step: API_INDEX_JXA_EOF heredoc not found in scripts/install.sh');
  process.exit(1);
}
const apiToIndex = new Function(`${m[1]}\nreturn apiToIndex;`)() as (
  rels: unknown[],
  asset: string,
) => string;
const digest = `sha256:${'0'.repeat(64)}`;
const cases = JSON.parse(
  readFileSync('internal/releaseline/testdata/next_step.json', 'utf8'),
) as Case[];
for (const [i, c] of cases.entries()) {
  const g = golden[i];
  if (!g || g.name !== c.name) {
    console.error(`check-install-step: golden out of order at "${c.name}"; regenerate it`);
    bad++;
    continue;
  }
  // Same dedupe and digests as releaseline's TestIndexGolden.
  const seen = new Set<string>();
  const rels = (c.releases ?? [])
    .filter((r) => !seen.has(r.tag_name) && seen.add(r.tag_name))
    .map((r) => ({
      ...r,
      assets: ((r as { assets?: object[] }).assets ?? []).map((a) => ({ ...a, digest })),
    }));
  const got = apiToIndex(rels, 'app.dmg');
  if (got !== g.index) {
    console.error(
      `check-install-step: "${c.name}": apiToIndex differs from FormatIndex\n${got}\nvs\n${g.index}`,
    );
    bad++;
  }
}
if (bad > 0) process.exit(1);
console.log(
  `check-install-step: awk stepping agrees on ${checked} cases, apiToIndex on ${cases.length}`,
);
