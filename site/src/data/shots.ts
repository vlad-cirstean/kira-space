import type { IconName } from './icons';

export interface ShotDef {
  readonly id: string;
  readonly title: string;
  readonly alt: string;
  readonly caption: string;
  readonly icon: IconName;
  readonly label: string;
}

// Files: screenshots/framed/<id>.png and screenshots/<id>-detail.png (bun run screenshots).
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
];
