/**
 * The pure half of `SearchResults.vue` — grouping (local branches / remote branches / tags /
 * commits), per-section caps and the two honesty footers, kept out of the template exactly as
 * `refListModel.ts` keeps `RefsPopover.vue`'s own fold out of *its* template. Nothing here is
 * Vue-reactive: every input is already-unwrapped data.
 */
import type { SearchField, SearchScope } from '@kira/git-core';
import type { RefRow } from '@kira/git-ipc';
import { type HostWordsSource, hostWords, type MessageKey, t } from '@theme/i18n';
import type { CommitHit, RefHit, SearchRunResult } from '../state/search.ts';
import { capItems, REF_LIST_SECTION_CAP } from './refListModel.ts';

/** One row in the dropdown, tagged by which half of `SearchState` it came from — the discriminant
 *  `SearchResults.vue`'s click/keyboard-select handler switches on to decide whether a hit is a
 *  `RefHit` or a `CommitHit` without a second lookup. `id` is a stable DOM id: what `:id` renders
 *  on the option and what `aria-activedescendant` (on `SearchBox.vue`'s own input — the element
 *  that holds focus, per the ARIA combobox pattern this dropdown follows) points at. */
export type SearchOption =
  | { readonly kind: 'ref'; readonly id: string; readonly hit: RefHit }
  | { readonly kind: 'commit'; readonly id: string; readonly hit: CommitHit };

function refOptionId(ref: RefRow): string {
  return `git-search-option-ref-${ref.refname}`;
}

function commitOptionId(hit: CommitHit): string {
  return `git-search-option-commit-${hit.sha}`;
}

interface SearchResultsSection {
  readonly title: string;
  readonly options: readonly SearchOption[];
  readonly hiddenCount: number;
}

export interface SearchResultsModel {
  /** Refs first (§7.8's own order), grouped and labelled by kind so a tag and a branch of the
   *  same name are never confused — the empty ones are omitted entirely rather than rendered as a
   *  section with nothing in it. */
  readonly sections: readonly SearchResultsSection[];
  /** Every visible option, refs-then-commits, section order preserved — what `SearchBox.vue`'s
   *  own arrow-key handler walks; a flat list because "next option" crosses section boundaries. */
  readonly flatOptions: readonly SearchOption[];
  /** "Searched N of M loaded commits" — set only when the client-side time box (hard part 4)
   *  actually fired; a scan that reached every loaded row has nothing to disclose. */
  readonly loadedFooter: string | undefined;
  /** "N more matches in history" — set only when the tail's own result cap truncated a
   *  larger total. There is no wire support for paging past it (`search.run` takes a `limit`, not
   *  a cursor), so this is disclosure, not an invitation to load more — narrowing the query is
   *  the only way to see a hit past the cap. */
  readonly tailFooter: string | undefined;
  /** Set for `tail?.kind === 'unsupportedPattern'` — a tail outcome that is neither
   *  silence nor a smaller `ok` — and when the tail request itself failed (`tailError`). The
   *  loaded-commit half is unaffected and still complete; this notice is what tells a user why a
   *  `regex`-mode pattern's tail (and therefore any body-only match) is missing. */
  readonly tailNotice: string | undefined;
}

/** A hit's own field list, rendered as one short label — "matched: body", "matched: sha" — for
 *  a hit that matched somewhere other than the subject/ref-name. `undefined` when `subject`/
 *  `refName` is present, since that is the row's own visible text and needs no label pointing at
 *  it. */
const FIELD_LABEL_KEYS = {
  subject: 'common.field.subject.label',
  body: 'common.body.label',
  authorName: 'common.field.author.label',
  authorEmail: 'git.search.field.authorEmail.label',
  committerName: 'git.commit.committer.label',
  committerEmail: 'git.search.field.committerEmail.label',
  sha: 'git.search.field.sha.label',
  refName: 'common.field.name.label',
  tagAnnotation: 'git.search.field.tagMessage.label',
  changeRequestNumber: 'git.search.field.changeRequestNumber.label',
  changeRequestTitle: 'git.search.field.changeRequestTitle.label',
} as const satisfies Record<SearchField, MessageKey>;

export function fieldLabel(
  fields: readonly SearchField[],
  host?: HostWordsSource | null,
): string | undefined {
  const other = fields.filter((f) => f !== 'subject' && f !== 'refName');
  if (other.length === 0) return undefined;
  const { changeRequestShort } = hostWords(host);
  return other.map((f) => t(FIELD_LABEL_KEYS[f], { changeRequestShort })).join(', ');
}

const SEARCH_SECTION_CAP = REF_LIST_SECTION_CAP;

function buildRefSection(title: string, hits: readonly RefHit[]): SearchResultsSection | undefined {
  if (hits.length === 0) return undefined;
  const { visible, hiddenCount } = capItems(hits, SEARCH_SECTION_CAP);
  return {
    title,
    hiddenCount,
    options: visible.map((hit) => ({ kind: 'ref', id: refOptionId(hit.ref), hit }) as const),
  };
}

function buildCommitSection(hits: readonly CommitHit[]): SearchResultsSection | undefined {
  if (hits.length === 0) return undefined;
  const { visible, hiddenCount } = capItems(hits, SEARCH_SECTION_CAP);
  return {
    title: t('common.module.commits.label'),
    hiddenCount,
    options: visible.map((hit) => ({ kind: 'commit', id: commitOptionId(hit), hit }) as const),
  };
}

export interface SearchResultsInput {
  readonly scope: SearchScope;
  readonly refHits: readonly RefHit[];
  readonly commitHits: readonly CommitHit[];
  /** `LoadedScanResult`, read only for `complete`/`scannedRows` — kept as loosely typed as the
   *  two fields this model actually reads so a caller need not import `@kira/git-core` just
   *  to satisfy this signature. */
  readonly loaded: { readonly complete: boolean; readonly scannedRows: number } | undefined;
  readonly loadedRowCount: number;
  readonly tail: SearchRunResult | undefined;
  /** `SearchState.tailError`. */
  readonly tailError?: string | undefined;
}

export function buildSearchResultsModel(input: SearchResultsInput): SearchResultsModel {
  const sections: SearchResultsSection[] = [];
  if (input.scope !== 'commits') {
    const branches = input.refHits.filter((h) => h.ref.kind === 'branch');
    const remotes = input.refHits.filter((h) => h.ref.kind === 'remoteBranch');
    const tags = input.refHits.filter((h) => h.ref.kind === 'tag');
    for (const section of [
      buildRefSection(t('common.module.branches.label'), branches),
      buildRefSection(t('git.picker.remoteBranches.label'), remotes),
      buildRefSection(t('git.tag.list.label'), tags),
    ]) {
      if (section) sections.push(section);
    }
  }
  if (input.scope !== 'refs') {
    const commitSection = buildCommitSection(input.commitHits);
    if (commitSection) sections.push(commitSection);
  }

  const loaded = input.loaded;
  const loadedFooter =
    loaded !== undefined && !loaded.complete
      ? t('git.search.loadedFooter.status', {
          scanned: loaded.scannedRows,
          count: input.loadedRowCount,
        })
      : undefined;

  const tail = input.tail;
  const tailFooter =
    tail?.kind === 'ok' && tail.truncated
      ? t('git.search.tailFooter.status', { count: tail.total - tail.hits.length })
      : undefined;
  const tailNotice =
    tail?.kind === 'unsupportedPattern'
      ? tail.message
      : input.tailError !== undefined
        ? t('git.search.tailFailed.error', { error: input.tailError })
        : undefined;

  return {
    sections,
    flatOptions: sections.flatMap((section) => section.options),
    loadedFooter,
    tailFooter,
    tailNotice,
  };
}
