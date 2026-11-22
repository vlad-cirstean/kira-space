import type { MessageKey } from '@theme/i18n/types';
import { artifactStatusTone } from '../artifacts/status';
import type { Tone } from '../board/actions';
import type { Artifact, ArtifactFound, Task } from '../wire';

export interface LinkItem {
  key: string;
  /** Artifact title, else its ref; never a bare number. */
  label: string;
  /** Full title, else the label. */
  title: string;
  repo: string;
  /** Who reported it: `jira`, `host`, `git`, `agent`, `user`, `task`, `notes`. */
  source: string;
  /** What the inline edit starts with. */
  editText: string;
  url: string;
  /** The key the in-app view looks up when there is no url: artifact ref, Jira key, host ref. */
  ref: string;
  kind: Artifact['kind'];
  /** `jira` and `host` are the task's own link fields; `stored` rows are the user's own; `found` links
   *  come out of notes and handovers and are never main. */
  origin: 'jira' | 'host' | 'stored' | 'detected' | 'found';
  /** Where a found link appears. */
  foundIn?: ArtifactFound[];
  artifactId?: string;
  /** The Kira session that reported it; '' = user, detected or unknown. */
  sessionId: string;
  status?: string;
  tone?: Tone;
}

const norm = (url: string): string => url.trim().replace(/\/+$/, '');
const BARE_REF = /^[#!]\d+$/;

function artifactLabel(a: Artifact): string {
  if (a.title) return a.title;
  if (a.ref) return BARE_REF.test(a.ref) && a.repo ? `${a.repo}${a.ref}` : a.ref;
  return a.url;
}

/** The free text a stored row edits as: `title url`, the bare url, or the title/ref of a urlless row. */
function storedEditText(a: Artifact): string {
  if (!a.url) return a.title || a.ref;
  return a.title && a.title !== a.url ? `${a.title} ${a.url}` : a.url;
}

function fromArtifact(a: Artifact): LinkItem {
  const stored = a.id !== '';
  const found = a.source === 'notes';
  const detectedKey = `detected:${a.kind}:${a.url}:${a.ref}:${a.title}:${a.repo}`;
  return {
    key: stored ? `stored:${a.id}` : found ? `found:${a.url}` : detectedKey,
    label: artifactLabel(a),
    title: a.title || artifactLabel(a),
    repo: a.repo,
    source: a.source,
    editText: stored ? storedEditText(a) : a.title,
    url: a.url,
    ref: a.ref,
    kind: a.kind,
    origin: stored ? 'stored' : found ? 'found' : 'detected',
    foundIn: found ? (a.foundIn ?? []) : undefined,
    artifactId: stored ? a.id : undefined,
    sessionId: a.sessionId,
    status: a.status || undefined,
    tone: a.status ? artifactStatusTone(a.status) : undefined,
  };
}

export type LinkCategory = 'issue' | 'changeRequest' | 'misc' | 'related';

const LINK_CATEGORIES: readonly LinkCategory[] = ['issue', 'changeRequest', 'misc', 'related'];

export const CATEGORY_LABEL = {
  issue: 'space.settings.updates.issue.title',
  changeRequest: 'space.ade.details.group.changeRequest.label',
  misc: 'space.ade.details.group.misc.label',
  related: 'space.ade.details.group.related.label',
} as const satisfies Record<LinkCategory, MessageKey>;

export const CATEGORY_ICON = {
  issue: 'issues',
  changeRequest: 'git-pull-request',
  misc: 'link',
  related: 'note',
} as const satisfies Record<LinkCategory, string>;

export const KINDS: readonly Artifact['kind'][] = [
  'issue',
  'changeRequest',
  'branch',
  'deployment',
  'doc',
  'link',
];

/** A found link is related whatever its kind (the kind still draws its icon). */
function linkCategory(item: LinkItem): LinkCategory {
  if (item.origin === 'found') return 'related';
  if (item.kind === 'issue' || item.kind === 'changeRequest') return item.kind;
  return 'misc';
}

/** Own: slots, detected change requests and issues, rows whose URL is in `ownUrls`. */
function isOwn(a: Artifact, ownSet: ReadonlySet<string>): boolean {
  if (a.source === 'notes') return false;
  if (a.id === '' && (a.kind === 'changeRequest' || a.kind === 'issue')) return true;
  return a.url !== '' && ownSet.has(norm(a.url));
}

/** Links bucketed by category (issue, change request, misc, related; every bucket present, maybe empty).
 *  Order within a bucket: the task's own links (Jira and host slots, detected change requests and
 *  issues, rows whose URL is in `ownUrls`), then others, then detected branches (only with
 *  `branches`); input order inside each. Related holds the links found in notes and handovers only. */
export function linkGroups(
  task: Task,
  artifacts: readonly Artifact[],
  opts: { branches?: boolean; ownUrls?: readonly string[] } = {},
): { id: LinkCategory; items: LinkItem[] }[] {
  const own: LinkItem[] = [];
  const jira = task.jira;
  if (jira) {
    own.push({
      key: 'jira',
      label: jira.key,
      title: jira.key,
      repo: '',
      source: 'jira',
      editText: jira.url || jira.key,
      url: jira.url,
      ref: jira.key,
      kind: 'issue',
      origin: 'jira',
      sessionId: '',
    });
  }
  if (task.hostUrl) {
    own.push({
      key: 'host',
      label: task.hostLink?.ref || task.hostUrl,
      title: task.hostLink?.ref || task.hostUrl,
      repo: '',
      source: 'host',
      editText: task.hostUrl,
      url: task.hostUrl,
      ref: task.hostLink?.ref ?? '',
      kind: task.hostLink?.kind ?? 'link',
      origin: 'host',
      sessionId: '',
    });
  }
  const seen = new Set(own.map((m) => norm(m.url)).filter(Boolean));
  const ownSet = new Set((opts.ownUrls ?? []).map(norm).filter(Boolean));
  const others: LinkItem[] = [];
  const branches: LinkItem[] = [];
  const found: LinkItem[] = [];
  for (const a of artifacts) {
    const detectedBranch = a.kind === 'branch' && a.id === '';
    if (detectedBranch && !opts.branches) continue;
    if (a.url && seen.has(norm(a.url))) continue;
    // The tracker's own row for the Jira key the task already shows.
    if (a.kind === 'issue' && jira && (a.title === jira.key || a.ref === jira.key)) continue;
    if (a.url) seen.add(norm(a.url));
    if (a.source === 'notes') found.push(fromArtifact(a));
    else if (detectedBranch) branches.push(fromArtifact(a));
    else (isOwn(a, ownSet) ? own : others).push(fromArtifact(a));
  }
  const all = [...own, ...others, ...branches, ...found];
  return LINK_CATEGORIES.map((id) => ({
    id,
    items: all.filter((item) => linkCategory(item) === id),
  }));
}
