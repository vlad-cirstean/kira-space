// Drives scripts/install.sh's resolve_release and download_release against a local fake GitHub
// (verify-packaging S17): the release index, the download redirects and the API fallback, with the
// request log proving the normal path never touches the API. The script is sourced with
// KIRA_INSTALL_SOURCE_ONLY=1 and its GH_WEB/GH_API/CURL_PROTO globals pointed at the fake, so no
// real GitHub traffic and no change to a real run's URLs.
import { createHash } from 'node:crypto';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const ASSET = 'app.dmg';
const INDEX_ASSET = 'kira-space-releases.txt';
const hasOsascript = Bun.which('osascript') !== null;

const sha = (b: Uint8Array) => createHash('sha256').update(b).digest('hex');
const body = (tag: string) => new TextEncoder().encode(`disk image of ${tag}`);
const row = (tag: string, edit?: { size?: number; digest?: string }) =>
  `${tag} ${ASSET} ${edit?.size ?? body(tag).length} sha256:${edit?.digest ?? sha(body(tag))}`;
const indexOf = (...rows: string[]) => `# kira-release-index 1\n${rows.join('\n')}\n`;

interface Reply {
  status: number;
  headers?: Record<string, string>;
  body?: string;
}
interface Fake {
  index: Reply;
  api: Reply;
  assetStatus: number;
}

let fake: Fake;
let requests: string[] = [];

const server = Bun.serve({
  port: 0,
  hostname: '127.0.0.1',
  fetch(req) {
    const path = new URL(req.url).pathname;
    requests.push(path);
    const reply = (r: Reply) =>
      new Response(r.body ?? '', { status: r.status, headers: r.headers });
    if (path === `/o/r/releases/latest/download/${INDEX_ASSET}`) {
      if (fake.index.status !== 200) return reply(fake.index);
      return new Response(null, { status: 302, headers: { Location: '/cdn/index' } });
    }
    if (path === '/cdn/index') return reply(fake.index);
    const dl = new RegExp(`^/o/r/releases/download/(v[0-9.]+)/${ASSET}$`).exec(path);
    if (dl) {
      if (fake.assetStatus !== 200) return new Response('', { status: fake.assetStatus });
      return new Response(null, { status: 302, headers: { Location: `/cdn/${dl[1]}` } });
    }
    const cdn = /^\/cdn\/(v[0-9.]+)$/.exec(path);
    if (cdn?.[1]) return new Response(body(cdn[1]));
    if (path === '/api/repos/o/r/releases') return reply(fake.api);
    return new Response('', { status: 500 });
  },
});

const apiRelease = (tag: string) => ({
  tag_name: tag,
  draft: false,
  prerelease: false,
  assets: [
    { name: ASSET, state: 'uploaded', size: body(tag).length, digest: `sha256:${sha(body(tag))}` },
  ],
});

interface Case {
  name: string;
  index?: Reply;
  api?: Reply;
  assetStatus?: number;
  req?: string;
  cur?: string;
  self?: boolean;
  skip?: boolean;
  wantOk?: boolean;
  wantTag?: string;
  wantErr?: string[];
  wantApi?: number;
}

const ok = (b: string): Reply => ({ status: 200, body: b });
const cases: Case[] = [
  {
    name: 'latest from the index, no API',
    index: ok(indexOf(row('v0.3.20'), row('v0.3.22'))),
    wantOk: true,
    wantTag: 'v0.3.22',
    wantApi: 0,
  },
  {
    name: '--tag in the index',
    index: ok(indexOf(row('v0.3.20'), row('v0.3.22'))),
    req: 'v0.3.20',
    wantOk: true,
    wantTag: 'v0.3.20',
    wantApi: 0,
  },
  {
    name: '--tag absent from the index',
    index: ok(indexOf(row('v0.3.20'), row('v0.3.22'))),
    req: 'v0.3.21',
    wantErr: ['not a published release'],
    wantApi: 0,
  },
  {
    name: 'self-update steps one minor',
    index: ok(indexOf(row('v0.3.9'), row('v0.3.14'), row('v0.4.0'))),
    cur: '0.3.9',
    self: true,
    wantOk: true,
    wantTag: 'v0.3.14',
    wantApi: 0,
  },
  {
    name: 'sha256 mismatch',
    index: ok(indexOf(row('v0.3.22', { digest: '0'.repeat(64) }))),
    wantErr: ['sha256'],
  },
  {
    name: 'size mismatch',
    index: ok(indexOf(row('v0.3.22', { size: 5 }))),
    wantErr: ['size'],
  },
  {
    name: 'malformed index',
    index: ok('# kira-release-index 1\nv0.3.22 only-two-fields\n'),
    wantErr: ['malformed'],
  },
  {
    name: 'index 500 does not fall back to the API',
    index: { status: 500 },
    wantErr: ["couldn't reach"],
    wantApi: 0,
  },
  {
    name: 'disk image 404',
    index: ok(indexOf(row('v0.3.22'))),
    assetStatus: 404,
    wantErr: ['not a published release'],
  },
  {
    name: 'index 404, API 403 rate limited',
    index: { status: 404 },
    api: { status: 403, headers: { 'x-ratelimit-remaining': '0' } },
    wantErr: ['rate limit', 'HTTP 403', 'x-ratelimit-remaining: 0'],
    wantApi: 1,
  },
  {
    name: 'index 404, API 429 is not retried',
    index: { status: 404 },
    api: { status: 429 },
    wantErr: ['rate limit', 'HTTP 429'],
    wantApi: 1,
  },
  {
    name: 'index 404, API listing',
    index: { status: 404 },
    api: ok(JSON.stringify([apiRelease('v0.3.21'), apiRelease('v0.3.20')])),
    skip: !hasOsascript,
    wantOk: true,
    wantTag: 'v0.3.21',
    wantApi: 1,
  },
];

// Values reach the script as env vars, never spliced into its source.
const driver = `
. scripts/install.sh
GH_WEB="http://127.0.0.1:$T_PORT/o/r"
GH_API="http://127.0.0.1:$T_PORT/api/repos/o/r"
CURL_PROTO="=http"
ASSET="${ASSET}"
INDEX_ASSET="${INDEX_ASSET}"
APP_NAME="Kira Space"
NOTIFY_FD="$T_NOTIFY"
LOG="$T_WORK/install.log"
WORK="$T_WORK"
resolve_release "$T_REQ" "$T_CUR" "$T_SELF"
download_release
printf 'TAG=%s\\n' "$TAG"
`;

let failed = 0;
let ran = 0;
for (const c of cases) {
  if (c.skip) {
    console.log(`check-install-resolve: skipped "${c.name}" (no osascript on this machine)`);
    continue;
  }
  fake = {
    index: c.index ?? { status: 404 },
    api: c.api ?? { status: 500 },
    assetStatus: c.assetStatus ?? 200,
  };
  requests = [];
  const work = mkdtempSync(join(tmpdir(), 'check-install-resolve-'));
  try {
    const proc = Bun.spawn(['sh', '-c', driver], {
      stdout: 'pipe',
      stderr: 'pipe',
      env: {
        PATH: process.env.PATH ?? '/usr/bin:/bin',
        HOME: work,
        KIRA_INSTALL_SOURCE_ONLY: '1',
        T_PORT: String(server.port),
        T_WORK: work,
        T_REQ: c.req ?? '',
        T_CUR: c.cur ?? '',
        T_SELF: c.self ? '1' : '0',
        T_NOTIFY: c.self ? '3' : '',
      },
    });
    const [out, err, code] = await Promise.all([
      new Response(proc.stdout).text(),
      new Response(proc.stderr).text(),
      proc.exited,
    ]);
    const problems: string[] = [];
    if (c.wantOk) {
      if (code !== 0) problems.push(`exit ${code}`);
      if (!out.includes(`TAG=${c.wantTag}`))
        problems.push(`want TAG=${c.wantTag}, got ${out.trim()}`);
      if (!existsSync(join(work, 'app.dmg'))) problems.push('no app.dmg downloaded');
    } else {
      if (code !== 1) problems.push(`exit ${code}, want 1`);
      for (const w of c.wantErr ?? []) if (!err.includes(w)) problems.push(`stderr lacks "${w}"`);
    }
    if (c.wantApi !== undefined) {
      const n = requests.filter((p) => p.startsWith('/api/')).length;
      if (n !== c.wantApi) problems.push(`${n} API requests, want ${c.wantApi}`);
    }
    ran++;
    if (problems.length > 0) {
      failed++;
      console.error(`check-install-resolve: "${c.name}": ${problems.join('; ')}\n${err}`);
    }
  } finally {
    rmSync(work, { recursive: true, force: true });
  }
}
server.stop(true);
if (failed > 0) process.exit(1);
console.log(`check-install-resolve: ${ran} cases passed`);
