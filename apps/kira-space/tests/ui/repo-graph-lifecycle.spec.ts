import { expect, test } from './fixtures';
import { freezeClock, runCoalesceWindow } from './support/clock';
import { gitStreamEmit, gitStreamRequests, installGitStreamMock } from './support/gitStreamMock';
import {
  manyRowShas,
  manyRows,
  openPortGraphWithFile,
  PORT_CONTROL,
  PORT_REPO,
  refsAtTip,
  singleRowChunks,
} from './support/gitUiPortFixtures';
import { IPC } from './support/ipcChannels';
import { repoRowById, repoTab } from './support/repoRows';
import type { ControlSnapshot } from './support/types';

// P67b §2/§3/§9: the two reported bugs, as regressions. Both need the real @kira/git-ui mount
// (RepoGraphView.vue/RepoReviewView.vue) actually reaching a live bridge — installGitStreamMock's
// own doc comment explains why its socket double starts CONNECTING and only opens on a later
// macrotask, and why send() now throws on a CONNECTING socket: without both properties, either bug
// would pass here even with the pre-fix code (a vacuous regression test, the plan's own warning).

const REPO = {
  id: 'repo-lifecycle-1',
  name: 'lifecycle-repo',
  root: '/tmp/lifecycle-repo',
  repoId: '/tmp/lifecycle-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path: 'a.ts' },
    response: {
      kind: 'found',
      text: 'export const a = 1;\n',
      bytes: 21,
      limitBytes: 8 * 1024 * 1024,
      language: 'typescript',
    },
  },
];

// P72 §7: the pinned graph tab now renders in `tab-strip-pinned`, a sibling of `tab-strip-row`
// rather than a descendant of it — `tab-strip-wrapper` is the common ancestor of both.

// `repo-graph-host`/`repo-review-host` are this app's OWN mount-point <div>s (RepoGraphView.vue/
// RepoReviewView.vue) — always present the instant the tab/segment renders, regardless of whether
// the git-ui bundle mounted inside them ever finishes bootstrapping. `connection-state` is
// @kira/git-ui's own BridgeClient.connectionState, exposed as text (App.vue/ReviewView.vue both
// carry it) — 'connecting' forever (never 'connected', never 'error') is exactly what a hung
// request over a silently-dropped-send dead socket looks like, so this is the assertion that
// proves the bootstrap succeeded rather than merely that a container div exists. Scoped
// to its own host: Review stays mounted via v-show once activated, so both a graph tab and
// an activated Review segment can carry their own `connection-state` span at the same time.
function connectionStateIn(page: import('@playwright/test').Page, hostTestId: string) {
  return page.locator(`[data-testid="${hostTestId}"] [data-testid="connection-state"]`);
}

// A cold mount reached the graph only after a manual Retry, because the git stream
// channel sent app.init before the socket's own open ack — deterministic on a fresh repo open, not
// flaky. The fix gates post() on the open ack instead of sending synchronously (streamChannel.ts).
// This is the FIRST-EVER open of a repo workspace on a fresh page.
test('bug 2 regression: a cold repo open reaches the graph with no boot-retry click', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  // Must land before the repo workspace ever opens — gitTransportFor is lazy, first triggered by
  // RepoGraphView.vue's own mount, which the repo-row click below causes.
  await installGitStreamMock(page, REPO.repoId);

  await repoRowById(page, REPO.id).click();

  await expect(page.locator('[data-testid="repo-graph-host"]')).toBeVisible();
  await expect(connectionStateIn(page, 'repo-graph-host')).toHaveText('connected');
  await expect(page.locator('[data-testid="boot-error"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="boot-retry"]')).toHaveCount(0);
});

// Closing a file tab disposes that mount's lease (inline blame) while the graph stays live. The
// dispose must release only its own registrations: a lease that closed the shared socket would
// leave the graph unable to auto-refresh. Each mount holds a per-mount lease
// (repo/git/transport.ts).
test("bug 1 regression: disposing another mount's lease leaves the graph live", async ({
  relaunch,
}) => {
  const page = await openPortGraphWithFile(relaunch, {
    file: 'a.ts',
    fileActive: false,
    chunks: singleRowChunks(manyRows(3)),
    results: { 'refs.list': refsAtTip(manyRowShas(0)) },
    control: [
      ...PORT_CONTROL,
      {
        channel: IPC.codeWorkspaceReadFile,
        args: { id: PORT_REPO.id, path: 'a.ts' },
        response: {
          kind: 'found',
          text: 'export const a = 1;\n',
          bytes: 21,
          limitBytes: 8 * 1024 * 1024,
          language: 'typescript',
        },
      },
    ],
  });
  await expect(connectionStateIn(page, 'repo-graph-host')).toHaveText('connected');

  await repoTab(page, 'repo-file').click();
  await expect(page.locator('[data-testid="repo-file-editor"]')).toBeVisible();
  await repoTab(page, 'repo-file').hover();
  await repoTab(page, 'repo-file').locator('[data-testid="tab-close"]').click();
  await expect(page.locator('[data-testid="repo-file-editor"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="repo-graph-host"]')).toBeVisible();

  const before = (await gitStreamRequests(page)).filter((m) => m === 'graph.refresh').length;
  await freezeClock(page);
  await gitStreamEmit(page, 'repo.changed', { repoId: PORT_REPO.repoId, kind: 'refsChanged' });
  await runCoalesceWindow(page);
  await expect
    .poll(async () => (await gitStreamRequests(page)).filter((m) => m === 'graph.refresh').length)
    .toBe(before + 1);
  await expect(connectionStateIn(page, 'repo-graph-host')).toHaveText('connected');
  await expect(page.locator('[data-testid="boot-error"]')).toHaveCount(0);
});
