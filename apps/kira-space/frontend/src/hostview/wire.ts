// Wire types of HostViewService (apps/kira-space/internal/bridge/hostview.go) and the githost
// detail contract (internal/githost/detail.go).

interface HostInfo {
  id: string;
  label: string;
  changeRequest: string;
  changeRequestNoun: string;
  changeRequestShort: string;
  cli: string;
}

export interface ChangeRequestTarget {
  kind: 'changeRequest';
  url: string;
  provider: string;
  host: HostInfo;
  repo: string;
  number: number;
}

interface TrackerTerms {
  label: string;
  issue: string;
  cli: string;
}

export interface IssueTarget {
  kind: 'issue';
  /** '' when the signed-in site was unknown at open. */
  url: string;
  provider: string;
  tracker: TrackerTerms;
  key: string;
}

export type HostViewTarget = ChangeRequestTarget | IssueTarget;

export interface HostViewOpen {
  key: string;
  opened: boolean;
}

interface HostStatus {
  kind: string;
  reason?: string;
  host?: string;
  account?: string;
  site?: string;
}

export type ReviewerState = 'approved' | 'changesRequested' | 'rejected' | 'commented' | 'pending';
export type CheckState = 'success' | 'failure' | 'error' | 'pending' | 'skipped' | 'neutral';
type ThreadKind = 'comment' | 'review' | 'code';

interface Person {
  login: string;
  name?: string;
}

interface Label {
  name: string;
  color?: string;
}

interface Reviewer {
  person: Person;
  team?: boolean;
  state: ReviewerState;
  required?: boolean;
}

export interface Check {
  name: string;
  state: CheckState;
  url?: string;
  detail?: string;
  required?: boolean;
}

interface Comment {
  id: string;
  author: Person;
  body: string;
  createdAt: number;
  updatedAt?: number;
  url?: string;
  /** Client only: an optimistic comment the host has not confirmed. */
  pending?: boolean;
}

export interface Thread {
  id: string;
  kind: ThreadKind;
  reviewState?: ReviewerState;
  path?: string;
  line?: number;
  resolved?: boolean;
  outdated?: boolean;
  comments: Comment[];
  commentsTruncated?: boolean;
}

interface DiffStats {
  commits: number;
  files: number;
  additions: number;
  deletions: number;
}

type ChangeRequestState = 'open' | 'draft' | 'merged' | 'closed';

export interface ChangeRequestDetail {
  number: number;
  title: string;
  url: string;
  state: ChangeRequestState;
  headRef: string;
  baseRef: string;
  updatedAt: number;
  reviewDecision?: string;
  checksState?: string;
  author: Person;
  body: string;
  createdAt: number;
  closedAt?: number;
  labels: Label[];
  assignees: Person[];
  reviewers: Reviewer[];
  checks: Check[];
  checksTruncated?: boolean;
  threads: Thread[];
  threadsTruncated?: boolean;
  stats?: DiffStats;
  mergeable?: 'mergeable' | 'conflicting';
}

type HostViewResultKind = 'ok' | 'unavailable' | 'gone';

export interface HostViewChangeRequest {
  kind: HostViewResultKind;
  detail?: ChangeRequestDetail;
  status: HostStatus;
  compose?: HostViewCompose;
}

/** What the window's composer may do (HostViewCompose in hostviewcompose.go). */
export interface HostViewCompose {
  maxBodyBytes: number;
  replyKinds: ThreadKind[];
}

export interface HostViewPosted {
  kind: HostViewResultKind;
  status?: HostStatus;
  reason?: string;
}

interface IssuePerson {
  id: string;
  name: string;
}

export interface IssueComment {
  id: string;
  author: IssuePerson;
  body: string;
  createdAt: number;
  updatedAt?: number;
  url?: string;
  /** Client only: an optimistic comment the host has not confirmed. */
  pending?: boolean;
}

export interface IssueDetail {
  ref: { provider: string; key: string; url: string; site: string };
  title: string;
  status: string;
  statusCategory: string;
  type?: string;
  priority?: string;
  resolution?: string;
  assignee?: IssuePerson;
  reporter?: IssuePerson;
  labels: string[];
  components: string[];
  createdAt: number;
  updatedAt: number;
  dueDate?: string;
  parent?: { key: string; title?: string };
  description: string;
  comments: IssueComment[];
  commentsTotal: number;
  commentsTruncated?: boolean;
  commentsOmitted?: boolean;
  summary: { known: boolean; count: number; state: string; providers: string[] };
}

export interface HostViewIssue {
  kind: HostViewResultKind;
  detail?: IssueDetail;
  reason?: string;
  compose?: HostViewCompose;
}
