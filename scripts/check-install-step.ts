// Replays internal/releaseline/testdata/next_step.json (the Go NextStep table) through the JXA
// twin embedded in scripts/install.sh, so the shell and Go stepping rules cannot drift
// (verify-packaging S16).
import { readFileSync } from 'node:fs';

interface Case {
  name: string;
  current: string;
  releases: unknown[] | null;
  target: string;
}

const script = readFileSync('scripts/install.sh', 'utf8');
const m = /<<'STEP_JXA_EOF'\n([\s\S]*?)\nSTEP_JXA_EOF\n/.exec(script);
if (!m?.[1]) {
  console.error('check-install-step: STEP_JXA_EOF heredoc not found in scripts/install.sh');
  process.exit(1);
}
const nextStep = new Function(`${m[1]}\nreturn nextStep;`)() as (
  current: string,
  rels: unknown[],
  asset: string,
) => string;

const cases = JSON.parse(
  readFileSync('internal/releaseline/testdata/next_step.json', 'utf8'),
) as Case[];
let bad = 0;
for (const c of cases) {
  const got = nextStep(c.current, c.releases ?? [], 'app.dmg');
  const want = c.target === '' ? '' : `v${c.target}`;
  if (got !== want) {
    console.error(
      `check-install-step: "${c.name}": install.sh picks '${got}', Go table wants '${want}'`,
    );
    bad++;
  }
}
if (bad > 0) process.exit(1);
console.log(
  `check-install-step: install.sh stepping agrees with ${cases.length} releaseline cases`,
);
