import type { IconName } from './icons';
import type { ShotId } from './shotCapture';

export interface ShotDef {
  readonly id: ShotId;
  readonly title: string;
  readonly alt: string;
  readonly caption: string;
  readonly icon: IconName;
  readonly label: string;
}

// Site copy per shot. How and where each shot is captured: shotCapture.ts (bun run screenshots).
export const SHOTS: readonly ShotDef[] = [
  {
    id: 'studio-data-grid',
    title: 'Data grid',
    alt: 'Kira Space data grid showing an orders table with one row selected.',
    caption: 'Data grid. Filter, sort, edit and commit rows.',
    icon: 'table',
    label: 'Data grid',
  },
  {
    id: 'studio-sql-console',
    title: 'SQL console',
    alt: 'Kira Space SQL console with a join query and its result grid.',
    caption: 'SQL console. Run a statement or the whole script.',
    icon: 'code',
    label: 'SQL console',
  },
  {
    id: 'studio-api',
    title: 'Api module',
    alt: 'Kira Space Api module showing an HTTP request and its JSON response.',
    caption: 'Api module. HTTP and gRPC requests with a JSON response.',
    icon: 'globe',
    label: 'Api module',
  },
  {
    id: 'space-git-graph',
    title: 'Git graph',
    alt: 'Kira Space commit graph with branches, tags and the commit detail pane open.',
    caption: 'Git graph. Branches, tags and commit detail.',
    icon: 'git-branch',
    label: 'Git graph',
  },
  {
    id: 'space-agents-board',
    title: 'Agents board',
    alt: 'Kira Space agents board with tasks across stages and live runs.',
    caption: 'Agents board. Tasks across stages, runs live.',
    icon: 'robot',
    label: 'Agents board',
  },
  {
    id: 'space-memory',
    title: 'Memory',
    alt: 'Kira Space memory list showing each fact with its origin and scope.',
    caption: 'Memory. Every fact shows origin and scope.',
    icon: 'lightbulb',
    label: 'Memory',
  },
  {
    id: 'studio-grid-row-menu',
    title: 'Row menu',
    alt: 'Kira Space grid context menu with copy, duplicate, revert and delete row actions.',
    caption: 'Row menu. Copy, duplicate, revert or delete the selected rows.',
    icon: 'table',
    label: 'Row menu',
  },
  {
    id: 'space-commit-detail',
    title: 'Commit detail',
    alt: 'Kira Space commit detail pane with the message and the changed files as a tree.',
    caption: 'Commit detail. Message and changed files as a tree.',
    icon: 'git-branch',
    label: 'Commit detail',
  },
];
