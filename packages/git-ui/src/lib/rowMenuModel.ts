/**
 * The row context menus' pure half. What is present, what is absent, and what is
 * present-but-disabled-with-a-reason is exactly the thing a later phase edits (the exit table
 * grows this same menu), so it lives as one testable table
 * (`buildRowMenu`/`buildRefMenu`) rather than template `v-if` conditionals.
 */
import { canRunOp, describeInProgress } from '@kira/git-core';
import type {
  DecorationRef,
  InProgressOperation,
  OpRequest,
  RefKind,
  StashEntry,
} from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { compareNatural } from '@workbench/behaviour/match';
// P131 Part 2 §3.3: MenuItem/MenuSection live in this package's own lib/menuModel.ts (a copy
// of the retired Kui context-menu model -- P131 Part 3
// §6.1) -- re-exported from here so every existing importer of `./rowMenuModel.ts` keeps working
// unchanged.
import type { MenuItem, MenuSection } from './menuModel.ts';
import { applyMenuLabel, originLabel } from './stashListModel.ts';
import { renderText } from './text.ts';

export type { MenuSection };

function gatedItem(
  id: string,
  label: string,
  opKind: OpRequest['kind'],
  inProgress: InProgressOperation | null,
  icon?: string,
  danger?: boolean,
): MenuItem {
  const allowed = canRunOp(inProgress, opKind);
  return {
    id,
    label,
    disabled: !allowed,
    disabledReason:
      allowed || inProgress === null ? undefined : renderText(describeInProgress(inProgress)),
    icon,
    danger,
  };
}

function plainItem(id: string, label: string, icon?: string): MenuItem {
  return { id, label, disabled: false, disabledReason: undefined, icon };
}

export interface CommitMenuContext {
  readonly sha: string;
  readonly decorations: readonly DecorationRef[];
  readonly inProgress: InProgressOperation | null;
}

/**
 * The per-commit menu: checkout (detached, gated same as the picker's own checkout — gating is
 * scoped to the op kind, not to where it was invoked from), create branch/tag here (never gated
 * — git does not refuse either mid-op), revert this commit (gated). Reset and cherry-pick, both
 * gated the same way, are placed after revert. Copy sha/copy message reuse the
 * `clipboardActions.ts`.
 */
export function buildRowMenu(ctx: CommitMenuContext): MenuSection[] {
  const mutating: MenuItem[] = [
    gatedItem(
      'checkoutDetached',
      t('git.menu.checkoutDetached.label'),
      'checkout',
      ctx.inProgress,
      'check',
    ),
    plainItem('createBranchHere', t('git.menu.newBranch.label'), 'git-branch'),
    plainItem('createTagHere', t('git.menu.newTag.label'), 'tag'),
    // worktreeAdd is not in GATED_OP_KINDS (git-core/src/model/operation.ts) — canRunOp
    // returns true for it unconditionally, so plainItem (not gatedItem, which would gate nothing)
    // is the honest call. Creating a worktree during a merge/rebase touches neither the sequencer
    // state nor the current worktree.
    plainItem('createWorktreeHere', t('git.worktree.create.label'), 'multiple-windows'),
    gatedItem(
      'revertThisCommit',
      t('git.menu.revertCommit.label'),
      'revert',
      ctx.inProgress,
      'history',
      true,
    ),
    gatedItem(
      'resetToThisCommit',
      t('git.menu.resetToCommit.label'),
      'reset',
      ctx.inProgress,
      'debug-step-back',
      true,
    ),
    gatedItem(
      'cherryPickThisCommit',
      t('git.menu.cherryPickCommit.label'),
      'cherryPick',
      ctx.inProgress,
      'git-commit',
    ),
  ];
  return [
    { items: mutating },
    {
      items: [
        plainItem('copySha', t('git.menu.copySha.label'), 'copy'),
        plainItem('copyMessage', t('git.menu.copyCommitMessage.label'), 'copy'),
      ],
    },
  ];
}

/**
 * `docs/plans/P7.md` W13 — the read-only per-commit menu of the review view's row: copy sha and
 * copy message *only* (§6.8's
 * "Read-only" — no checkout/branch/tag/revert, never gated on `canRunOp`, since none of these
 * items are operations git could refuse mid-op; C10 hides the mutating items rather than
 * disabling them, `docs/HISTORY.md (v1.5 C10)` §4.2 layer 3). Kept
 * separate from `buildRowMenu` rather than that function gated down to nothing by a flag, so a
 * reader never has to check "which of these does the review row actually get" against a table of
 * conditions — there is no table, there is a second, smaller function.
 */
export function buildReadOnlyRowMenu(): MenuSection[] {
  return [
    {
      items: [
        plainItem('copySha', t('git.menu.copySha.label'), 'copy'),
        plainItem('copyMessage', t('git.menu.copyCommitMessage.label'), 'copy'),
      ],
    },
  ];
}

/**
 * `FileTree.vue`'s right-click "Copy path" menu — one item, deliberately
 * not the two-affordance duplication `ReviewCommitRow.vue` used to carry, built directly against
 * this package's own `lib/menuModel.ts`'s `MenuItem` type from the start,
 * matching every other menu-building function in this file. This is now the one copy-
 * path affordance in every tree, not only the review-styled ones it started out scoped to.
 *
 * `goToFileEnabled` adds "Go to file" — absent (not disabled) for a caller with no
 * commit sha to resolve (`FileTree.vue`'s own `sha` prop is optional).
 */
export function buildFileRowMenu(goToFileEnabled: boolean): MenuSection[] {
  const items: MenuItem[] = [];
  if (goToFileEnabled) items.push(plainItem('goToFile', t('common.goToFile.label'), 'go-to-file'));
  items.push(plainItem('copyPath', t('common.action.copyPath.label'), 'copy'));
  return [{ items }];
}

export interface RefMenuContext {
  readonly kind: RefKind;
  readonly shortName: string;
  readonly isHead: boolean;
  /** Set (a remote name) when a lightweight/annotated tag also exists on that remote — the delete
   *  entry's own asymmetry warning (§7.9: deleting locally does not delete on the remote) is
   *  worded differently depending on whether this is even reachable for this tag. */
  readonly knownRemotes: readonly string[];
  readonly inProgress: InProgressOperation | null;
  /** This branch's own stack membership — `undefined` when the caller has no stack
   *  view mounted at all (a tag/remoteBranch row, or a host with no `StackState`), matching
   *  `buildRowMenu`'s own "absent, not disabled" convention for a capability that is not
   *  there. Meaningless for `kind !== 'branch'` (a tag/remote-tracking ref is never a stack
   *  member) and ignored for those. */
  readonly stack?: {
    /** This branch has a recorded stack parent (D1) — gates "Remove from stack"/"Restack this
     *  stack"/"Go to parent branch". */
    readonly hasParent: boolean;
    /** Some other branch records this one as ITS parent — gates "Go to child branch". */
    readonly hasChild: boolean;
  };
}

/**
 * "every mutating action is available from a context menu on the row it applies to", read
 * literally for a ref: the row a branch/tag operation applies to is the ref itself (the picker's
 * own row, or — once wired — the graph's own ref badge), not the commit it happens to point at.
 * Branch: checkout/rename/delete. Tag: checkout/delete/push/delete-on-remote (the last two only
 * where a remote is known — there is no `remotes.list` endpoint of its own, so
 * `knownRemotes` is derived from the already-loaded `remoteBranches` list rather than a second
 * round trip; see `remoteNamesFrom` below).
 */
export function buildRefMenu(ctx: RefMenuContext): MenuSection[] {
  if (ctx.kind === 'tag') {
    const items: MenuItem[] = [
      gatedItem(
        'checkoutRef',
        t('common.action.checkOut.label'),
        'checkout',
        ctx.inProgress,
        'check',
      ),
      gatedItem(
        'deleteRef',
        t('common.action.delete.label'),
        'tagDelete',
        ctx.inProgress,
        'trash',
        true,
      ),
    ];
    // tagPush/tagDeleteRemote are the two
    // OpRequest kinds opTable has never served (RunOp's write path has no askpass wiring — see
    // commands.ts's own MutatingEntry doc comment for the tracked "move both to remote.run"
    // fix) — these two items were offered enabled anyway, so a click always rejected with
    // E_UNKNOWN_METHOD and the rejection was discarded, a silent no-op with no toast, no error, no
    // announcement. Disabled with a reason until that server-side move lands, rather than
    // offering an action that can never succeed.
    const TAG_REMOTE_OPS_UNSUPPORTED = t('git.menu.notSupported.hint');
    for (const remote of ctx.knownRemotes) {
      items.push({
        id: `pushRef:${remote}`,
        label: t('git.toolbar.push.tooltip', { remote }),
        disabled: true,
        disabledReason: TAG_REMOTE_OPS_UNSUPPORTED,
        icon: 'repo-push',
      });
      items.push({
        id: `deleteRemoteRef:${remote}`,
        label: t('git.menu.deleteOnRemote.label', { remote }),
        disabled: true,
        disabledReason: TAG_REMOTE_OPS_UNSUPPORTED,
        icon: 'trash',
        danger: true,
      });
    }
    return [{ items }];
  }
  // branch or remoteBranch — a remote-tracking ref itself is read-only here (its only action is
  // the checkout the picker's row already offers); rename/delete apply to a local branch only.
  // "Review branch changes" is offered for both — it is a read, never
  // gated on `canRunOp` (the in-progress gate is about operations git would refuse), and is
  // absent for a tag entirely (a tag is a point, not a line of development).
  if (ctx.kind === 'remoteBranch') {
    return [
      {
        items: [
          gatedItem(
            'checkoutRef',
            t('common.action.checkOut.label'),
            'checkout',
            ctx.inProgress,
            'check',
          ),
          plainItem('reviewBranch', t('git.review.start.title'), 'diff-multiple'),
          plainItem('createWorktreeHere', t('git.worktree.create.label'), 'multiple-windows'),
        ],
      },
    ];
  }
  const items: MenuItem[] = [
    gatedItem(
      'checkoutRef',
      t('common.action.checkOut.label'),
      'checkout',
      ctx.inProgress,
      'check',
    ),
    plainItem('renameRef', t('common.action.rename.label'), 'edit'),
    plainItem('reviewBranch', t('git.review.start.title'), 'diff-multiple'),
    plainItem('createWorktreeHere', t('git.worktree.create.label'), 'multiple-windows'),
  ];
  // git refuses to delete the branch you are currently on — not one of §7.11's gated op kinds
  // (the gate is scoped to what an in-progress *operation* blocks), so this is its own, simpler
  // disablement with its own reason.
  items.push(
    ctx.isHead
      ? {
          id: 'deleteRef',
          label: t('common.action.delete.label'),
          disabled: true,
          disabledReason: t('git.menu.currentBranch.hint'),
          icon: 'trash',
          danger: true,
        }
      : gatedItem(
          'deleteRef',
          t('common.action.delete.label'),
          'branchDelete',
          ctx.inProgress,
          'trash',
          true,
        ),
  );
  const sections: MenuSection[] = [{ items }];
  // A second section, present only when the caller has a stack view mounted
  // (`ctx.stack !== undefined`) — never gated on `canRunOp` (stackSet/restack are config/rebase
  // writes, not one of the in-progress-blocked kinds), matching "Review branch changes"'
  // own un-gated posture just above.
  if (ctx.stack !== undefined) {
    const stackItems: MenuItem[] = [
      plainItem(
        'stackSetParent',
        ctx.stack.hasParent
          ? t('git.menu.stackChangeParent.label')
          : t('git.stack.setParent.tooltip'),
        'list-tree',
      ),
    ];
    if (ctx.stack.hasParent) {
      stackItems.push(plainItem('stackRemove', t('git.stack.remove.tooltip'), 'close'));
      stackItems.push(plainItem('stackRestack', t('git.menu.stackRestack.label'), 'sync'));
      stackItems.push(
        plainItem('stackGoToParent', t('git.menu.stackGoToParent.label'), 'arrow-up'),
      );
    }
    if (ctx.stack.hasChild) {
      stackItems.push(
        plainItem('stackGoToChild', t('git.menu.stackGoToChild.label'), 'arrow-down'),
      );
    }
    sections.push({ items: stackItems });
  }
  return sections;
}

/**
 * `docs/plans/P9.md` W14: §7.6's per-stash menu — Apply, Pop, Drop, Branch (all gated on
 * `canRunOp`, exactly as every other stack-mutating action is; `stashDrop` reads as un-gated in
 * practice only because `core`'s own `GATED_OP_KINDS` never lists it — see `model/operation.ts`'s
 * own comment) and Show (a read, never gated, mirroring `buildRowMenu`'s copy actions). One
 * section — there is no clipboard-conditional second section here, unlike `buildRowMenu`, since a
 * stash row's sha is already visible via `StashList.vue`/the badge's own title, not a menu item.
 *
 * G28 D5 widens this by two behaviors, both keyed off `entry`/`currentBranch`: the Apply label
 * reads `"Apply here (from <origin>)"` for a cross-branch entry (`applyMenuLabel`), and Pop is
 * OMITTED entirely for one — popping a stash from another branch drops it from the stack on
 * success, which for the "I'll want this back on `main` later" case is a trap; Apply (which keeps
 * the entry, probe P1) is the correct verb across a branch boundary, and the menu says so instead
 * of leaving both and hoping the user picks correctly. G28 D13 also adds "Save to global stash…"
 * — the row-level entry point `StashDialog.vue`'s fourth mode needs to pre-select THIS entry as
 * its source (D10 step 3, "promote an existing entry"), never gated (saving never fails the way a
 * git write can; the source stays untouched either way, D10's own copy-never-drop guarantee).
 */
export function buildStashMenu(
  inProgress: InProgressOperation | null,
  entry: StashEntry,
  currentBranch: string | null | undefined,
): MenuSection[] {
  const crossBranch = originLabel(entry, currentBranch) !== undefined;
  const items: MenuItem[] = [
    gatedItem(
      'stashApply',
      applyMenuLabel(entry, currentBranch),
      'stashApply',
      inProgress,
      'diff-added',
    ),
  ];
  if (!crossBranch) {
    items.push(
      gatedItem(
        'stashPop',
        t('git.dialog.stash.popConfirm.confirm'),
        'stashPop',
        inProgress,
        'export',
      ),
    );
  }
  items.push(
    gatedItem('stashDrop', t('git.menu.stashDrop.label'), 'stashDrop', inProgress, 'trash', true),
    gatedItem(
      'stashBranch',
      t('git.menu.stashBranch.label'),
      'stashBranch',
      inProgress,
      'git-branch',
    ),
    plainItem('stashSaveGlobal', t('git.stash.saveGlobal.tooltip'), 'archive'),
    plainItem('stashShow', t('git.menu.stashShow.label'), 'eye'),
  );
  return [{ items }];
}

/**
 * The global bucket's own row menu — Apply here, Create branch from this…, Show changes, Remove
 * from global stash. NO Pop, NO Drop: both are position-addressed by necessity and a global entry
 * has no stack position at all — Apply-plus-Remove is the honest pair for a keep-forever entry.
 * `stashApply`/`stashBranch` reuse the SAME `OpRequest['kind']` gates
 * `buildStashMenu` uses (the op kinds themselves are unchanged for a global entry — only how
 * the server resolves the source sha differs); `globalStashRemove` is its own gate.
 */
export function buildGlobalStashMenu(
  inProgress: InProgressOperation | null,
  entry: StashEntry,
  currentBranch: string | null | undefined,
): MenuSection[] {
  return [
    {
      items: [
        gatedItem(
          'stashApply',
          applyMenuLabel(entry, currentBranch),
          'stashApply',
          inProgress,
          'diff-added',
        ),
        gatedItem(
          'stashBranch',
          t('git.menu.globalStashBranch.label'),
          'stashBranch',
          inProgress,
          'git-branch',
        ),
        plainItem('stashShow', t('git.menu.stashShow.label'), 'eye'),
        gatedItem(
          'globalStashRemove',
          t('git.menu.globalStashRemove.label'),
          'globalStashRemove',
          inProgress,
          'trash',
          true,
        ),
      ],
    },
  ];
}

/** Distinct remote names implied by an already-loaded `remoteBranches` list (`origin/main` →
 *  `origin`) — see `buildRefMenu`'s own doc comment on why this stands in for a `remotes.list`
 *  endpoint P6 does not have. */
export function remoteNamesFrom(remoteBranchNames: readonly string[]): string[] {
  const names = new Set<string>();
  for (const name of remoteBranchNames) {
    const slash = name.indexOf('/');
    if (slash > 0) names.add(name.slice(0, slash));
  }
  return [...names].sort(compareNatural);
}

/** The remote Fetch/Pull/Push target: the current branch's upstream remote when it names a known
 *  remote, else `origin` when present, else the first (alphabetical) name. */
export function defaultRemoteFor(
  knownRemotes: readonly string[],
  upstream: string | undefined,
): string | undefined {
  const fromUpstream = knownRemotes.find((name) => upstream?.startsWith(`${name}/`));
  if (fromUpstream !== undefined) return fromUpstream;
  return knownRemotes.includes('origin') ? 'origin' : knownRemotes[0];
}
