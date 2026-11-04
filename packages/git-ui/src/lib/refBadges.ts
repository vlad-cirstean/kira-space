/**
 * Pure DOM-builder functions for the ref badges shown inline at the start
 * of the message cell — not a Vue SFC, because a SlickGrid formatter is synchronous and
 * mounting a component per badge per row would mean a Vue app instance and lifecycle per badge,
 * created and torn down as rows scroll, orders of magnitude more expensive than the badges
 * themselves for nothing. The same reasoning
 * applies to `columns.ts`'s other formatters and to the graph column: nothing inside a grid row
 * is a Vue component.
 *
 * The single source for "is this row a stash" is `DecorationRef`'s `stash` kind
 * (`store.decorationAt(row)`, `packages/git/src/parse/log.ts`'s `parseDecorationToken`) — never a
 * second heuristic over the subject line, which an ordinary commit could coincidentally match.
 * The graph-column stash node shape reads the exact same decoration, so the two never disagree.
 *
 * Split deliberately in two: `badgeSpecFor`/`planBadges` are pure functions over data (no DOM),
 * unit-tested directly below in `tests/unit/ui/refBadges.spec.ts`; `buildRefBadges` and its
 * private `build*Element` helpers touch `document` and so, per this repo's own precedent (no
 * jsdom/happy-dom is wired into `bun:test` — confirmed, not assumed), are exercised only by
 * the Playwright pass rather than here.
 */
import type { DecorationRef } from '@kira/git-core';
import type { ChangeRequestRecord } from '@kira/git-ipc';
import { t } from '@theme/i18n';
import { BADGE_ICONS } from '../icons/index.ts';
import { changeRequestBadgeClass, type RefBadgeVariant, refBadgeClass } from './badgeClass.ts';
import { changeRequestStateLabel } from './changeRequestStateLabel.ts';

// §6.2: badge text truncates at ~190px (full name in the tooltip); the icon keeps its size.
/** A Git ref name, not prose. */
const HEAD_REF_NAME = 'HEAD';
const BADGE_LABEL_CLASS = 'min-w-0 max-w-47.5 truncate';

/** §6.2: "a row with more than three badges collapses the overflow into a +N badge". */
const MAX_VISIBLE_BADGES = 3;

export interface BadgeSpec {
  readonly icon: string;
  /** Studio `Badge` tone for this kind; never a colour value (B4: colours live in the theme layer). */
  readonly variant: RefBadgeVariant;
  readonly text: string;
  readonly isCurrentBranch: boolean;
  /** Set only for `branch`/`remoteBranch` — the two kinds
   *  `CommitGrid.vue`'s ref-badge hit-test and `App.vue`'s ref context menu care about. A tag/HEAD
   *  badge has no review action and no rename/delete-from-the-graph menu, so it carries neither
   *  field — the hit-test's own `[data-ref-kind]` selector is exactly this distinction, made a DOM
   *  query rather than a second decoration check. Adds `"stash"`: a stash
   *  badge's own menu (`buildStashMenu`) needs the same hit-test seam, `refName` carrying
   *  `stash@{N}` rather than a ref name proper (there is no ref for anything but the top entry —
   *  see `parseDecorationToken`'s own doc comment). */
  readonly refKind: 'branch' | 'remoteBranch' | 'stash' | undefined;
  readonly refName: string | undefined;
}

/** The text used both on the badge itself and in a `+N` overflow badge's title list — one
 *  function so the two never disagree about what a decoration is "called". Exhaustive over
 *  `DecorationRef["kind"]`: a new kind is a compile error here, not a silently-dropped badge. */
export function badgeSpecFor(ref: DecorationRef): BadgeSpec {
  switch (ref.kind) {
    case 'branch':
      return {
        icon: BADGE_ICONS.localBranch,
        variant: 'ok',
        text: ref.name,
        isCurrentBranch: ref.isHead,
        refKind: 'branch',
        refName: ref.name,
      };
    case 'remoteBranch':
      return {
        icon: BADGE_ICONS.remoteBranch,
        variant: 'info',
        text: ref.name,
        isCurrentBranch: false,
        refKind: 'remoteBranch',
        refName: ref.name,
      };
    case 'tag':
      return {
        icon: BADGE_ICONS.tag,
        variant: 'default',
        text: ref.name,
        isCurrentBranch: false,
        refKind: undefined,
        refName: undefined,
      };
    case 'stash':
      // P9 W14: `stash@{N}` — same text a `StashEntry`'s own `index` renders everywhere else
      // (`StashList.vue`, `StashDialog.vue`) — not the bare "stash" P4 shipped, which could not
      // say *which* stash a badge on a non-`stash@{0}` row (P9 W12's own graph walk) belonged to.
      return {
        icon: BADGE_ICONS.stash,
        variant: 'default',
        text: `stash@{${ref.index}}`,
        isCurrentBranch: false,
        refKind: 'stash',
        refName: `stash@{${ref.index}}`,
      };
    case 'head':
      // Detached HEAD: the table describes the filled dot as a modifier on the *branch*
      // badge, but a detached HEAD has no branch decoration alongside it to modify — this is
      // the one case the table's wording does not literally cover. It gets the same local-branch
      // pill and dot, with the literal text "HEAD" standing in for a branch name that does not
      // exist, rather than going undecorated (which would make a detached-HEAD commit visually
      // indistinguishable from an ordinary one — the opposite of what this column is for).
      return {
        icon: BADGE_ICONS.localBranch,
        variant: 'warn',
        text: HEAD_REF_NAME,
        isCurrentBranch: true,
        refKind: undefined,
        refName: undefined,
      };
  }
}

interface OverflowSpec {
  /** The `N` in "+N" — the *hidden* count, not the row's total decoration count. */
  readonly count: number;
  /** Names every decoration on the row, not just the hidden ones — "a row with six decorations
   *  renders three plus +3 and its title names all six" (§6.2's own "Done when" wording). */
  readonly title: string;
}

interface BadgePlan {
  readonly visible: readonly BadgeSpec[];
  readonly overflow: OverflowSpec | null;
}

/** G26 D-4.11: a branch badge's own stack decoration — `stacked` uses the `info` tone, `stale`
 *  (meaningful only alongside `stacked`) the `warn` tone. No new `DecorationRef` kind; looked up
 *  separately by branch name from `columns.ts`'s `StackContext`, applied only to `branch` badges. */
export interface StackBadgeInfo {
  readonly stacked: boolean;
  readonly stale: boolean;
}

/** The pure "what to render" computation: which badges show, and what the overflow badge (if
 *  any) says — with no DOM touched, so this is what `tests/unit/ui/refBadges.spec.ts` exercises
 *  directly. `buildRefBadges` below is a thin DOM-construction layer over this. */
function planBadges(decorations: readonly DecorationRef[]): BadgePlan {
  const specs = decorations.map(badgeSpecFor);
  const visible = specs.slice(0, MAX_VISIBLE_BADGES);
  const overflow: OverflowSpec | null =
    specs.length > MAX_VISIBLE_BADGES
      ? {
          count: specs.length - MAX_VISIBLE_BADGES,
          title: specs.map((spec) => spec.text).join(', '),
        }
      : null;
  return { visible, overflow };
}

function buildBadgeElement(
  spec: BadgeSpec,
  stackInfoFor?: (branchName: string) => StackBadgeInfo | undefined,
): HTMLSpanElement {
  const badge = document.createElement('span');
  let variant = spec.variant;
  if (spec.refKind === 'branch' && spec.refName !== undefined) {
    const stackInfo = stackInfoFor?.(spec.refName);
    if (stackInfo?.stacked) variant = stackInfo.stale ? 'warn' : 'info';
  }
  const classes: string[] = [];
  // G-UX (item 1): a subtle ring on the current-branch badge itself. `ring-current` (not a fixed
  // tone like `ring-focus`): the ring must read as "this badge" however many tones exist
  // (ok/warn today), never a colour of its own that can clash with the badge's own text-{tone}.
  if (spec.isCurrentBranch) classes.push('outline outline-1 -outline-offset-1 outline-current');
  badge.className = refBadgeClass(variant, 'shrink min-w-0', ...classes);
  if (spec.icon === BADGE_ICONS.tag) badge.dataset.testid = 'badge-tag';
  // The full name lives in `data-kira-tip`, read by the one `AttributeTooltip`
  // CommitGrid.vue mounts over its grid host — a mouse-hover affordance independent of whether the
  // ~190px CSS truncation (`BADGE_LABEL_CLASS`) clips this particular badge's text. This file
  // is plain DOM code outside Vue, so it writes the attribute a real `Tooltip`/`TooltipTrigger`
  // would otherwise carry, rather than using either directly. No `aria-label` alongside it: the
  // visible label span below already gives this badge a real accessible name.
  badge.setAttribute('data-kira-tip', spec.text);

  // `docs/plans/P7.md` W14: the one seam `CommitGrid.vue`'s `handleContextMenu` hit-tests for
  // (`closest("[data-ref-kind]")`) — present only for `branch`/`remoteBranch` (see `BadgeSpec`'s
  // own doc comment on why a tag/stash/HEAD badge carries neither).
  if (spec.refKind !== undefined && spec.refName !== undefined) {
    badge.dataset.refKind = spec.refKind;
    badge.dataset.refName = spec.refName;
  }

  const icon = document.createElement('span');
  icon.className = `codicon shrink-0 codicon-${spec.icon}`;
  icon.dataset.testid = 'badge-icon';
  // Decorative: the visible label text (or, for the overflow badge, its title) already carries
  // the information — see this file's module doc on "no colour/glyph-only meaning" (§7.9/W14).
  icon.setAttribute('aria-hidden', 'true');
  badge.appendChild(icon);

  const label = document.createElement('span');
  label.className = BADGE_LABEL_CLASS;
  label.textContent = spec.text;
  badge.appendChild(label);

  if (spec.isCurrentBranch) {
    // A check glyph, not a plain dot — the dot was a 5×5px circle, easy to miss
    // next to the badge's own icon; a recognisable checkmark reads as "current" at a glance
    // without needing colour to carry the meaning. Still `role="img"` + `aria-label`, the same
    // "reliably reach the accessibility tree as a second, non-text signal" reasoning the dot it
    // replaces already established (the "no colour/shape-only meaning" still applies).
    const check = document.createElement('span');
    check.className = 'codicon codicon-check shrink-0 text-focus';
    check.setAttribute('role', 'img');
    check.setAttribute('aria-label', t('git.common.currentBranch.tooltip'));
    badge.appendChild(check);
  }

  return badge;
}

function buildOverflowBadge(overflow: OverflowSpec): HTMLSpanElement {
  const badge = document.createElement('span');
  badge.className = refBadgeClass('default', 'shrink-0');
  badge.setAttribute('data-kira-tip', overflow.title);
  badge.textContent = `+${overflow.count}`;
  return badge;
}

// ---------------------------------------------------------------------------------------
// The per-commit graph indicator — one badge per commit, never one per associated
// PR. Kept in this file (not a new module) since it is, structurally, a fifth badge kind sharing
// every one of `buildBadgeElement`'s conventions (`refBadgeClass`, an icon, `data-kira-tip`) —
// it just never goes through `badgeSpecFor`/`DecorationRef`, since a PR is not a ref decoration.
// ---------------------------------------------------------------------------------------

const PR_STATE_PRECEDENCE: Readonly<Record<ChangeRequestRecord['state'], number>> = {
  open: 0,
  draft: 1,
  merged: 2,
  closed: 3,
};

/** D9's own precedence rule: `open > draft > merged > closed`, ties broken by most recently
 *  updated — "which one PR does the graph indicator show when a commit is associated with more
 *  than one" (§10.8's own recommendation). `undefined` for an empty list, never thrown. */
export function pickBestChangeRequest(
  prs: readonly ChangeRequestRecord[],
): ChangeRequestRecord | undefined {
  if (prs.length === 0) return undefined;
  return [...prs].sort((a, b) => {
    const precedence = PR_STATE_PRECEDENCE[a.state] - PR_STATE_PRECEDENCE[b.state];
    return precedence !== 0 ? precedence : b.updatedAt - a.updatedAt || a.number - b.number;
  })[0];
}

/** Builds the graph indicator's own badge for one commit's associated PR(s), or `null` when there
 *  is nothing to show — every non-resolved-with-a-PR outcome (not-yet-resolved, in-flight,
 *  `disabled`, `unavailable`, or resolved-with-none) is exactly this `null` (the "inert,
 *  never noisy" rule): the caller never has to branch on which of those five it got.
 *  Never a real `<a href>` — under Wails an anchor click navigates the app's own window
 *  itself rather than opening a browser tab (there is no separate tab to open one in), the same
 *  defect class `CommitMeta.vue`/`RefsPopover.vue`/`StackList.vue`'s own PR links carried before
 *  the fix. A `<button data-change-request-number>` (click routed through
 *  `CommitGrid.vue`'s own `onClick` delegation to `DetailActions.openChangeRequest`, which composes
 *  and opens the URL host-side). The PR's state is carried as a border colour class, never as text
 *  (`changeRequestBadgeClass`) — width is scarce, and the tooltip already names the state in
 *  words. */
export function buildChangeRequestBadge(prs: readonly ChangeRequestRecord[]): HTMLElement | null {
  const best = pickBestChangeRequest(prs);
  if (best === undefined) return null;

  const badge = document.createElement('button');
  badge.className = changeRequestBadgeClass(best.state);
  badge.dataset.testid = 'badge-change-request';
  badge.type = 'button';
  // Not a Tab stop inside the roving-tabindex grid; Enter on it would toggle the detail pane.
  badge.tabIndex = -1;
  badge.dataset.changeRequestNumber = String(best.number);
  const state = changeRequestStateLabel(best.state);
  badge.setAttribute(
    'data-kira-tip',
    prs.length > 1
      ? [
          best.title,
          state,
          t('git.refBadge.changeRequestMore.tooltip', { count: prs.length - 1 }),
        ].join('\n')
      : `${best.title}\n${state}`,
  );

  const label = document.createElement('span');
  label.className = BADGE_LABEL_CLASS;
  label.textContent = `#${best.number}`;
  badge.appendChild(label);

  return badge;
}

/**
 * Builds the inline badge strip for one row's decorations, or `null` for a row with none — the
 * caller (`columns.ts`'s `messageFormatter`) skips the wrapper element entirely in that case
 * rather than inserting an empty, non-contributing `<span>` into every one of the tens of
 * thousands of ordinary rows a full history walk can produce.
 */
export function buildRefBadges(
  decorations: readonly DecorationRef[],
  stackInfoFor?: (branchName: string) => StackBadgeInfo | undefined,
): HTMLSpanElement | null {
  if (decorations.length === 0) return null;

  const plan = planBadges(decorations);
  const container = document.createElement('span');
  container.className = 'flex items-center gap-1 min-w-0';

  for (const spec of plan.visible) {
    container.appendChild(buildBadgeElement(spec, stackInfoFor));
  }
  if (plan.overflow !== null) container.appendChild(buildOverflowBadge(plan.overflow));

  return container;
}
