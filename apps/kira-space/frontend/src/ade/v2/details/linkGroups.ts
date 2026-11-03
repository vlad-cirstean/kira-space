import type { Artifact, Task } from '../wire';

export interface LinkItem {
  key: string;
  /** Artifact title, else its ref; never a bare number. */
  label: string;
  /** What the inline edit starts with. */
  editText: string;
  url: string;
  kind: Artifact['kind'];
  /** `jira` and `host` are the task's own link fields; `stored` rows can be renamed. */
  origin: 'jira' | 'host' | 'stored' | 'detected';
  artifactId?: string;
  /** The Kira session that reported it; '' = user, detected or unknown. */
  sessionId: string;
  status?: string;
}

const norm = (url: string): string => url.trim().replace(/\/+$/, '');
const BARE_REF = /^[#!]\d+$/;

function artifactLabel(a: Artifact): string {
  if (a.title) return a.title;
  if (a.ref) return BARE_REF.test(a.ref) && a.repo ? `${a.repo}${a.ref}` : a.ref;
  return a.url;
}

function fromArtifact(a: Artifact): LinkItem {
  const stored = a.id !== '';
  return {
    key: stored ? `stored:${a.id}` : `detected:${a.kind}:${a.url}:${a.ref}:${a.title}:${a.repo}`,
    label: artifactLabel(a),
    editText: a.title,
    url: a.url,
    kind: a.kind,
    origin: stored ? 'stored' : 'detected',
    artifactId: stored ? a.id : undefined,
    sessionId: a.sessionId,
    status: a.status || undefined,
  };
}

/** Main: the task's Jira and host links and its detected change requests. Related: every other
 *  artifact but branches (Main shows those), minus what Main already holds. */
export function linkGroups(
  task: Task,
  artifacts: readonly Artifact[],
): { main: LinkItem[]; related: LinkItem[] } {
  const main: LinkItem[] = [];
  const jira = task.jira;
  if (jira) {
    main.push({
      key: 'jira',
      label: jira.key,
      editText: jira.key,
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
  for (const a of artifacts) {
    if (a.kind === 'branch') continue;
    if (a.url && seen.has(norm(a.url))) continue;
    // The tracker's own row for the Jira key the task already shows.
    if (a.kind === 'issue' && jira && (a.title === jira.key || a.ref === jira.key)) continue;
    related.push(fromArtifact(a));
  }
  return { main, related };
}
