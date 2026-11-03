import { describe, expect, test } from 'bun:test';
import type {
  CheckoutPreflight,
  OpResult,
  PullPreflight,
  RefRow,
  RemoteOpResult,
  RevertPreflight,
  StashEntry,
  StatusSummary,
} from '@kira/git-ipc';
import { sleep } from '@workbench/testing/unit/async';
import { BridgeClient } from '../bridge/client.ts';
import { FakeTransport } from '../testing/fakeTransport.ts';
import { OpsState, remoteFromUpstreamRef } from './ops.ts';
import { RefsState } from './refs.ts';
import { RepoSettingsState } from './repoSettings.ts';

const REPO = '/repos/a';

const STATUS: StatusSummary = {
  head: { kind: 'branch', name: 'main' },
  upstream: { name: 'origin/main', ahead: 0, behind: 1 },
  counts: { staged: 0, unstaged: 0, untracked: 1, unmerged: 0 },
  isClean: false,
  dirtyPaths: ['untracked.txt'],
  dirtyTruncated: false,
  inProgress: null,
  autoFetch: null,
};

const PRE_EXISTING_STASH: StashEntry = {
  index: 0,
  sha: 'existing-stash-sha',
  baseSha: 'base-sha',
  baseSubject: 'an unrelated earlier stash',
  indexSha: 'index-sha',
  untrackedSha: undefined,
  message: "WIP on main: the user's own earlier stash",
  branch: 'main',
  timestamp: 0,
  fileCount: 1,
  includedUntracked: false,
  scope: 'stack',
  ref: '',
};

// #stashAndCarry used to treat stash.list's post-push entries[0] as "the entry stashAndCarry itself
// just pushed". When the worktree is dirty ONLY with untracked files, stashPush (called with
// includeUntracked: false) is a git no-op — "No local changes to save", exit 0 — so pushResult.ok
// is true but the stash stack is unchanged. If the user already had a stash of their own, that
// PRE-EXISTING entry is still entries[0], and the old code ran preflight.stashPop + op.run stashPop
// against it — silently applying and deleting a stash this pull never created.
describe('OpsState — #stashAndCarry (via runPull) never touches a pre-existing, unrelated stash', () => {
  test('an untracked-only no-op stashPush leaves a pre-existing stash alone', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const refs = new RefsState(bridge);
    const ops = new OpsState(bridge, refs, new RepoSettingsState(bridge));

    const preflight: PullPreflight = {
      strategy: 'merge',
      source: 'default',
      rebaseMerges: false,
      upstream: 'origin/main',
      ahead: 0,
      behind: 1,
      dirty: true,
      routes: [],
      blockers: ['dirtyNonFastForward'],
    };
    const pushResult: OpResult = {
      ok: true,
      error: undefined,
      undo: null,
      head: { kind: 'branch', name: 'main' },
      inProgress: null,
    };
    const pullResult: RemoteOpResult = {
      ok: true,
      error: undefined,
      updates: [],
      head: { kind: 'branch', name: 'main' },
      inProgress: null,
    };

    transport.onRequest = (method) => {
      switch (method) {
        case 'status.get':
          return STATUS;
        case 'undo.peek':
          return { slot: null };
        case 'remote.pullPreflight':
          return preflight;
        case 'stash.list':
          // Unchanged across every call: the no-op push never touches the stack.
          return { entries: [PRE_EXISTING_STASH] };
        case 'op.run':
          return pushResult;
        case 'remote.run':
          return pullResult;
        default:
          throw new Error(`unscripted request: ${method}`);
      }
    };
    ops.setRepoId(REPO);
    await sleep(); // let setRepoId's own fire-and-forget refreshStatus/refreshUndo settle first.

    const runPromise = ops.runPull('origin', 'main');
    // runPull's blockers.length > 0 branch awaits #confirmPull before doing anything else —
    // resolve it exactly as PullDialog.vue's own "Stash and pull" button would.
    await sleep();
    ops.resolvePullDialog(true);
    await runPromise;

    const stashPopAttempted = transport.calls.some(
      (c) =>
        c.method === 'preflight.stashPop' ||
        (c.method === 'op.run' && (c.params as { op: { kind: string } }).op.kind === 'stashPop'),
    );
    expect(stashPopAttempted).toBe(false);
    expect(ops.announcement.value).toBe('Pulled origin/main (merge)');
  });
});

// P168 Part 18 F14: a rejected request used to escape every op unannounced.
describe('OpsState — rejected requests are announced, not thrown', () => {
  function setUp(onRun: () => unknown) {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const refs = new RefsState(bridge);
    const ops = new OpsState(bridge, refs, new RepoSettingsState(bridge));
    transport.onRequest = (method) => {
      switch (method) {
        case 'status.get':
          return STATUS;
        case 'undo.peek':
          return { slot: null };
        case 'remote.pullPreflight':
          return {
            strategy: 'merge',
            source: 'default',
            rebaseMerges: false,
            upstream: 'origin/main',
            ahead: 0,
            behind: 1,
            dirty: true,
            routes: [],
            blockers: ['dirtyNonFastForward'],
          } satisfies PullPreflight;
        case 'stash.list':
          return { entries: [] };
        case 'op.run':
          return {
            ok: true,
            error: undefined,
            undo: null,
            head: { kind: 'branch', name: 'main' },
            inProgress: null,
          } satisfies OpResult;
        case 'remote.run':
          return onRun();
        default:
          throw new Error(`unscripted request: ${method}`);
      }
    };
    ops.setRepoId(REPO);
    return { ops, transport };
  }

  test('a stash-carry pull whose remote.run rejects says the changes are stashed', async () => {
    const { ops } = setUp(() => {
      throw new Error('socket closed');
    });
    await sleep();
    const run = ops.runPull('origin', 'main');
    await sleep();
    ops.resolvePullDialog(true);
    await run;
    expect(ops.announcement.value).toContain('Couldn’t pull.');
    expect(ops.announcement.value).toContain('socket closed');
    expect(ops.announcement.value).toContain('Your changes are stashed');
    expect(ops.busy.value).toBe(false);
  });

  test('a plain failing fetch announces and resolves', async () => {
    const { ops } = setUp(() => {
      throw new Error('socket closed');
    });
    await sleep();
    await ops.runFetch('origin');
    expect(ops.announcement.value).toContain('Couldn’t fetch.');
  });

  test('progress arriving after the remote op settles is ignored', async () => {
    const { ops, transport } = setUp(() => ({
      ok: true,
      error: undefined,
      updates: [],
      head: { kind: 'branch', name: 'main' },
      inProgress: null,
    }));
    await sleep();
    await ops.runFetch('origin');
    transport.emit('remote.progress', {
      repoId: REPO,
      phase: 'receiving',
      percent: 50,
      done: 1,
      total: 2,
      remote: true,
    });
    expect(ops.remoteProgress.value).toBeUndefined();
  });
});

describe('remoteFromUpstreamRef', () => {
  test('extracts the remote name from a remote-tracking upstream', () => {
    expect(remoteFromUpstreamRef('refs/remotes/origin/feature')).toBe('origin');
    expect(remoteFromUpstreamRef('refs/remotes/upstream/main')).toBe('upstream');
  });

  test('undefined for a purely local upstream (no remote to pull from)', () => {
    expect(remoteFromUpstreamRef('refs/heads/main')).toBeUndefined();
  });

  test('undefined for no upstream at all', () => {
    expect(remoteFromUpstreamRef(undefined)).toBeUndefined();
  });
});

// G-UX (item 3): "when checking out a branch that advanced, ask if I want to pull it too" —
// runCheckout's own post-switch confirm step.
describe('OpsState — post-checkout pull prompt', () => {
  function branchRow(overrides: Partial<RefRow> = {}): RefRow {
    return {
      refname: 'refs/heads/feature',
      kind: 'branch',
      shortName: 'feature',
      objectId: 'sha-feature',
      peeledObjectId: undefined,
      upstream: 'refs/remotes/origin/feature',
      track: { ahead: 0, behind: 3 },
      committerDate: 0,
      isHead: false,
      checkedOutIn: undefined,
      annotation: undefined,
      ...overrides,
    };
  }

  const CLEAN_CHECKOUT_PREFLIGHT: CheckoutPreflight = {
    target: { kind: 'branch', name: 'feature' },
    detaches: false,
    createsTracking: undefined,
    carried: [],
    blockers: [],
    verdict: 'clean',
    routes: [],
  };

  const CHECKOUT_RESULT: OpResult = {
    ok: true,
    error: undefined,
    undo: null,
    head: { kind: 'branch', name: 'feature' },
    inProgress: null,
  };

  function setUp(branches: readonly RefRow[]): {
    ops: OpsState;
    refs: RefsState;
    transport: FakeTransport;
  } {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const refs = new RefsState(bridge);
    const ops = new OpsState(bridge, refs, new RepoSettingsState(bridge));
    transport.onRequest = (method) => {
      switch (method) {
        case 'status.get':
          return STATUS;
        case 'undo.peek':
          return { slot: null };
        case 'preflight.checkout':
          return CLEAN_CHECKOUT_PREFLIGHT;
        case 'op.run':
          return CHECKOUT_RESULT;
        default:
          throw new Error(`unscripted request: ${method}`);
      }
    };
    ops.setRepoId(REPO);
    refs.branches.value = branches;
    return { ops, refs, transport };
  }

  test('prompts, and "Pull now" runs the pull against the branch\'s own upstream remote', async () => {
    const { ops, transport } = setUp([branchRow()]);
    // Script the pull half only once the checkout half has run — `remote.pullPreflight`/
    // `remote.run` are not requests the checkout-only fixture above answers.
    let pullPreflightRequested = false;
    const originalOnRequest = transport.onRequest;
    transport.onRequest = (method, params) => {
      if (method === 'remote.pullPreflight') {
        pullPreflightRequested = true;
        return {
          strategy: 'merge',
          source: 'default',
          rebaseMerges: false,
          upstream: 'origin/feature',
          ahead: 0,
          behind: 3,
          dirty: false,
          routes: [],
          blockers: [],
        } satisfies PullPreflight;
      }
      if (method === 'remote.run') {
        return {
          ok: true,
          error: undefined,
          updates: [],
          head: { kind: 'branch', name: 'feature' },
          inProgress: null,
        } satisfies RemoteOpResult;
      }
      return originalOnRequest(method, params);
    };

    await sleep();
    const runPromise = ops.runCheckout('feature', 'switch');
    // runCheckout's own post-switch prompt awaits #confirmPostCheckoutPull — resolve it exactly
    // as PostCheckoutPullDialog.vue's own "Pull now" button would, before awaiting the call's own
    // completion (it does not resolve until the dialog does).
    await sleep();
    expect(ops.pendingPostCheckoutPull.value).toEqual({
      branch: 'feature',
      remote: 'origin',
      upstreamShortName: 'origin/feature',
      behind: 3,
    });

    ops.resolvePostCheckoutPullDialog(true);
    await runPromise;

    expect(pullPreflightRequested).toBe(true);
    const pullCall = transport.calls.find((c) => c.method === 'remote.run');
    if (pullCall === undefined) throw new Error('expected a remote.run call');
    const pullParams = pullCall.params as { remote: string; branch: string };
    expect(pullParams.remote).toBe('origin');
    expect(pullParams.branch).toBe('feature');
  });

  test('"Not now" leaves the branch checked out without pulling', async () => {
    const { ops, transport } = setUp([branchRow()]);
    await sleep();
    const runPromise = ops.runCheckout('feature', 'switch');
    await sleep();

    expect(ops.pendingPostCheckoutPull.value).toBeDefined();
    ops.resolvePostCheckoutPullDialog(false);
    await runPromise;

    expect(ops.pendingPostCheckoutPull.value).toBeUndefined();
    expect(transport.calls.some((c) => c.method === 'remote.pullPreflight')).toBe(false);
  });

  test('no prompt when the branch is already up to date', async () => {
    const { ops } = setUp([branchRow({ track: { ahead: 0, behind: 0 } })]);
    await sleep();
    await ops.runCheckout('feature', 'switch');
    expect(ops.pendingPostCheckoutPull.value).toBeUndefined();
  });

  test('no prompt when the branch has no upstream', async () => {
    const { ops } = setUp([branchRow({ track: undefined, upstream: undefined })]);
    await sleep();
    await ops.runCheckout('feature', 'switch');
    expect(ops.pendingPostCheckoutPull.value).toBeUndefined();
  });

  test('no prompt when the upstream is gone', async () => {
    const { ops } = setUp([branchRow({ track: 'gone' })]);
    await sleep();
    await ops.runCheckout('feature', 'switch');
    expect(ops.pendingPostCheckoutPull.value).toBeUndefined();
  });

  test('no prompt on a detach, even to a target behind its own upstream', async () => {
    const { ops } = setUp([branchRow()]);
    await sleep();
    await ops.runCheckout('feature', 'detach');
    expect(ops.pendingPostCheckoutPull.value).toBeUndefined();
  });
});

// Every #confirm* dialog above (checkout,
// revert, reset, cherry-pick, stash apply/pop, pull, force push) opens a promise that only
// resolves once the user clicks a button — arbitrarily long real-world wall-clock time,
// during which App.vue's "Open in graph" can call OpsState.setRepoId() directly (it does not wait
// on `busy`, since revealCommitInGraph is independent of whatever operation is mid-confirm). Before
// this fix, resolving a stale dialog against the now-active repo would run op.run/#applyResult with
// the OLD repoId's preflight/route but write results into whatever OpsState now considers current —
// cross-repo data corruption. Exercises the pattern via runRevert; the same guard shape is applied
// identically at every other #confirm* call site.
describe('OpsState — a stale confirm dialog no-ops if the active repo changed while it was open', () => {
  test('runRevert never calls op.run when the repo changes before the revert dialog resolves', async () => {
    const OTHER_REPO = '/repos/b';
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const refs = new RefsState(bridge);
    const ops = new OpsState(bridge, refs, new RepoSettingsState(bridge));

    const preflight: RevertPreflight = {
      shas: ['sha1'],
      mainlineRequired: [],
      dirtyPaths: [],
      inProgress: null,
      prediction: { kind: 'clean' },
      predictedFor: 'sha1',
      detachedHead: false,
      verdict: 'willConflict',
      blockers: [],
    };

    transport.onRequest = (method) => {
      switch (method) {
        case 'status.get':
          return STATUS;
        case 'undo.peek':
          return { slot: null };
        case 'preflight.revert':
          return preflight;
        default:
          throw new Error(`unscripted request: ${method}`);
      }
    };
    ops.setRepoId(REPO);
    await sleep();

    const runPromise = ops.runRevert(['sha1']);
    // runRevert's willConflict verdict awaits #confirmRevert before doing anything else.
    await sleep();
    expect(ops.pendingRevert.value).toBeDefined();

    // The active repo changes while the dialog is still open — e.g. the user clicked "Open in
    // graph" on a commit from a different repo entirely.
    ops.setRepoId(OTHER_REPO);
    await sleep();

    // Now the (stale) dialog resolves, as if the user had just clicked its confirm button.
    ops.resolveRevertDialog({ mainline: undefined, noCommit: false });
    await runPromise;

    const revertAttempted = transport.calls.some(
      (c) => c.method === 'op.run' && (c.params as { op: { kind: string } }).op.kind === 'revert',
    );
    expect(revertAttempted).toBe(false);
  });
});

// P111: rebaseMerges travels with the preflight-resolved strategy; an explicit override always
// sends false, never derived here from strategy === 'rebase' -- the same pick-vs-ladder confusion
// already slipped past two review rounds (Part 15 F6, Part 16 F6) before it moved server-side.
describe('OpsState — runPull carries rebaseMerges from the preflight, not a client-side guess', () => {
  function setUp(): { ops: OpsState; transport: FakeTransport } {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const refs = new RefsState(bridge);
    const ops = new OpsState(bridge, refs, new RepoSettingsState(bridge));
    transport.onRequest = (method) => {
      switch (method) {
        case 'status.get':
          return STATUS;
        case 'undo.peek':
          return { slot: null };
        case 'remote.pullPreflight':
          return {
            strategy: 'rebase',
            source: 'pullConfig',
            rebaseMerges: true,
            upstream: 'origin/main',
            ahead: 0,
            behind: 1,
            dirty: false,
            routes: [],
            blockers: [],
          } satisfies PullPreflight;
        case 'remote.run':
          return {
            ok: true,
            error: undefined,
            updates: [],
            head: { kind: 'branch', name: 'main' },
            inProgress: null,
          } satisfies RemoteOpResult;
        default:
          throw new Error(`unscripted request: ${method}`);
      }
    };
    ops.setRepoId(REPO);
    return { ops, transport };
  }

  function remoteRunRebaseMerges(transport: FakeTransport): unknown {
    const call = transport.calls.find((c) => c.method === 'remote.run');
    return (call?.params as { rebaseMerges?: unknown } | undefined)?.rebaseMerges;
  }

  test("running the preflight-resolved strategy sends the preflight's own rebaseMerges", async () => {
    const { ops, transport } = setUp();
    await ops.runPull('origin', 'main');
    expect(remoteRunRebaseMerges(transport)).toBe(true);
  });

  test('an explicit override always sends rebaseMerges: false', async () => {
    const { ops, transport } = setUp();
    await ops.runPull('origin', 'main', 'rebase');
    expect(remoteRunRebaseMerges(transport)).toBe(false);
  });
});
