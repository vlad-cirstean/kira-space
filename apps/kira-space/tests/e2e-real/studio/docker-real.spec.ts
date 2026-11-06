import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { expect, test } from './fixtures';

// P232: Docker mode against the real engine. Skips without one (same message as the Go flow suite).
// The container is labelled for the harness sweep and removed in the test's own cleanup.

// bash image: the exec shell prefers bash, and busybox ash can swallow an Enter that follows its
// cursor-position query.
const IMAGE = 'mirror.gcr.io/library/bash:5.2';

function docker(...args: string[]): string {
  return execFileSync('docker', args, {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}

function engineError(): string | null {
  try {
    docker('version', '--format', '{{.Server.Version}}');
    return null;
  } catch (err) {
    return (err as Error).message.split('\n')[0];
  }
}

function dockerHost(): string | undefined {
  if (process.env.DOCKER_HOST) return process.env.DOCKER_HOST;
  try {
    return docker('context', 'inspect', '--format', '{{.Endpoints.docker.Host}}') || undefined;
  } catch {
    return undefined;
  }
}

const unavailable = engineError();
const host = dockerHost();
test.use({ serverEnv: host ? { DOCKER_HOST: host } : {} });

test('Docker mode lists a compose-labelled container, streams logs, execs and stops it', async ({
  kira,
}) => {
  test.skip(
    unavailable !== null,
    `docker: no engine (${unavailable}); start dockerd or colima, or unset KIRA_FLOW_DOCKER`,
  );
  const id = randomUUID().slice(0, 8);
  const project = `kiraflow-e2e-${id}`;
  const name = `kira-flow-e2e-${id}-web`;
  docker(
    'run',
    '-d',
    '--name',
    name,
    '--stop-timeout',
    '1',
    '--label',
    `kira.flowtest=e2e-${id}`,
    '--label',
    `com.docker.compose.project=${project}`,
    '--label',
    'com.docker.compose.service=web',
    IMAGE,
    'sh',
    '-c',
    'echo kira-ready; exec sleep 300',
  );
  try {
    const page = kira.window;
    await page.locator('[data-testid="mode-tab"][data-mode="docker"]').click();
    await expect(page.locator('[data-testid="docker-panel"]')).toBeVisible();

    await expect(
      page.locator('[data-testid="docker-group"]').filter({ hasText: project }),
    ).toBeVisible();
    const row = page.locator(`[data-testid="docker-row"][data-name="${name}"]`);
    await expect(row).toHaveAttribute('data-state', 'running');
    await row.click();
    await expect(page.locator('[data-testid="docker-container-detail"]')).toBeVisible();

    await page.locator('[data-testid="docker-tab-logs"]').click();
    await expect(
      page.locator('[data-testid="docker-log-line"]').filter({ hasText: 'kira-ready' }),
    ).toBeVisible();

    await page.locator('[data-testid="docker-tab-terminal"]').click();
    await page.locator('[data-testid="docker-exec-new"]').click();
    await expect(page.locator('.xterm-rows')).toBeVisible();
    await page.locator('.xterm-helper-textarea').focus();
    await page.keyboard.type('echo kira-$((1+1))', { delay: 0 });
    await expect(page.locator('.xterm-rows')).toContainText('echo kira-$((1+1))');
    await page.keyboard.press('Enter');
    await expect(page.locator('.xterm-rows')).toContainText('kira-2');

    await row.hover();
    await row.locator('[data-testid="docker-row-stop"]').click();
    await expect(row).toHaveAttribute('data-state', 'exited', { timeout: 20_000 });
  } finally {
    try {
      docker('rm', '-f', '-v', name);
    } catch {
      // already gone
    }
  }
});
