import type { MessageKey } from '@theme/i18n/types';
import { artifactStatusTone } from '../artifacts/status';
import type { Tone } from '../board/actions';
import type { Artifact, Task } from '../wire';

export interface LinkItem {
  key: string;
  /** Artifact title, else its ref; never a bare number. */
  label: string;
  /** Full title, else the label. */
  title: string;
  repo: string;
  /** Who reported it: `jira`, `host`, `git`, `agent`, `user`, `task`. */
  source: string;
  /** What the inline edit starts with. */
  editText: string;
  url: string;
  kind: Artifact['kind'];
  /** `jira` and `host` are the task's own link fields; `stored` rows are the user's own. */
  origin: 'jira' | 'host' | 'stored' | 'detected';
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
  return {
    key: stored ? `stored:${a.id}` : `detected:${a.kind}:${a.url}:${a.ref}:${a.title}:${a.repo}`,
    label: artifactLabel(a),
    title: a.title || artifactLabel(a),
    repo: a.repo,
    source: a.source,
    editText: stored ? storedEditText(a) : a.title,
    url: a.url,
    kind: a.kind,
    origin: stored ? 'stored' : 'detected',
    artifactId: stored ? a.id : undefined,
    sessionId: a.sessionId,
    status: a.status || undefined,
    tone: a.status ? artifactStatusTone(a.status) : undefined,
  };
}

export type LinkCategory = 'issue' | 'changeRequest' | 'misc';

export const LINK_CATEGORIES: readonly LinkCategory[] = ['issue', 'changeRequest', 'misc'];

export const CATEGORY_LABEL = {
  issue: 'space.settings.updates.issue.title',
  changeRequest: 'space.ade.details.group.changeRequest.label',
  misc: 'space.ade.details.group.misc.label',
} as const satisfies Record<LinkCategory, MessageKey>;

export const CATEGORY_ICON = {
  issue: 'issues',
  changeRequest: 'git-pull-request',
  misc: 'link',
} as const satisfies Record<LinkCategory, string>;

export const KINDS: readonly Artifact['kind'][] = [
  'issue',
  'changeRequest',
  'branch',
  'deployment',
  'doc',
  'link',
];

export function linkCategory(kind: Artifact['kind']): LinkCategory {
  if (kind === 'issue' || kind === 'changeRequest') return kind;
  return 'misc';
}

/** Links bucketed by category (issue, change request, misc; every bucket present, maybe empty).
 *  Order within a bucket: the task's Jira and host links, detected change requests, then every
 *  other artifact, minus what the first set already holds. Detected branches only with `branches`. */
export function linkGroups(
  task: Task,
  artifacts: readonly Artifact[],
  opts: { branches?: boolean } = {},
): { id: LinkCategory; items: LinkItem[] }[] {
  const main: LinkItem[] = [];
  const jira = task.jira;
  if (jira) {
    main.push({
      key: 'jira',
      label: jira.key,
      title: jira.key,
      repo: '',
      source: 'jira',
      editText: jira.url || jira.key,
      url: jira.url,
      kind: 'issue',
      origin: 'jira',
      sessionId: '',
    });
  }
  if (task.hostUrl) {
    main.push({
      key: 'host',
      label: task.hostLink?.ref || task.hostUrl,
      title: task.hostLink?.ref || task.hostUrl,
      repo: '',
      source: 'host',
      editText: task.hostUrl,
      url: task.hostUrl,
      kind: task.hostLink?.kind ?? 'link',
      origin: 'host',
      sessionId: '',
    });
  }
  const seen = new Set(main.map((m) => norm(m.url)).filter(Boolean));
  for (const a of artifacts) {
    if (a.kind !== 'changeRequest' || a.id !== '' || a.source !== 'host') continue;
    if (a.url && seen.has(norm(a.url))) continue;
    if (a.url) seen.add(norm(a.url));
    main.push(fromArtifact(a));
  }
  const related: LinkItem[] = [];
  const branches: LinkItem[] = [];
  for (const a of artifacts) {
    if (a.kind === 'branch' && a.id === '' && !opts.branches) continue;
    if (a.url && seen.has(norm(a.url))) continue;
    // The tracker's own row for the Jira key the task already shows.
    if (a.kind === 'issue' && jira && (a.title === jira.key || a.ref === jira.key)) continue;
    (a.kind === 'branch' && a.id === '' ? branches : related).push(fromArtifact(a));
  }
  const all = [...main, ...related, ...branches];
  return LINK_CATEGORIES.map((id) => ({
    id,
    items: all.filter((item) => linkCategory(item.kind) === id),
  }));
}
