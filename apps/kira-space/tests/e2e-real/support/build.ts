import { execFileSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { acquireBuildLock } from '@workbench/testing/e2eReal';

// The one real-backend binary both e2e-real tiers launch: a `go build -tags server` build of the
// merged app, plus the fake claude/gh agent. Idempotent and memoized per worker process; the
// cross-process build lock serializes concurrent workers.

export const ROOT_DIR = resolve(__dirname, '../../../../..');
export const APP_DIR = resolve(ROOT_DIR, 'apps/kira-space');
export const SERVER_BINARY = resolve(APP_DIR, 'bin/kira-space-server-test');
export const FAKE_AGENT_BINARY = resolve(APP_DIR, 'bin/fakeclaude');

// `scripts/setup.sh` installs `wails3` via `go install` and expects it on PATH afterward; set
// explicitly so the fixtures never depend on the invoking shell having done that.
export function envWithGoBin(): NodeJS.ProcessEnv {
  const extra = `${execFileSync('go', ['env', 'GOPATH'], { encoding: 'utf8' }).trim()}/bin`;
  const path = process.env.PATH ?? '';
  return { ...process.env, PATH: path.includes(extra) ? path : `${path}:${extra}` };
}

let prerequisitesReady: Promise<void> | undefined;

export function buildPrerequisites(): Promise<void> {
  prerequisitesReady ??= (async () => {
    const release = await acquireBuildLock();
    try {
      const env = envWithGoBin();
      const run = (cmd: string, args: string[], cwd = ROOT_DIR) =>
        execFileSync(cmd, args, { cwd, env, stdio: 'inherit' });
      run('sh', ['scripts/setup.sh']);
      // The hooks-enabled desktop build, which main.go's `//go:embed all:frontend/dist` picks up.
      run('bun', ['run', 'build:test']);
      if (!existsSync(resolve(APP_DIR, 'frontend/dist-mobile/index.html'))) {
        run('bun', ['run', 'build:mobile']);
      }
      await mkdir(resolve(APP_DIR, 'bin'), { recursive: true });
      run('go', ['build', '-tags', 'server', '-o', SERVER_BINARY, '.'], APP_DIR);
      run(
        'go',
        ['build', '-o', FAKE_AGENT_BINARY, '../../internal/flowtest/fakeagent/cmd/fakeclaude'],
        APP_DIR,
      );
    } finally {
      await release();
    }
  })();
  return prerequisitesReady;
}
