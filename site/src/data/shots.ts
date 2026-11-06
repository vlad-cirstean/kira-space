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
  {
    id: 'build-start-board',
    title: 'Plan board',
    alt: 'Kira Space plan board with the Export orders as CSV task planned today on the Feature workflow.',
    caption: 'Plan board. The task sits on today with its workflow and step.',
    icon: 'robot',
    label: 'Plan board',
  },
  {
    id: 'build-start-capture',
    title: 'New task',
    alt: 'Kira Space new task dialog with the title and Jira key filled in.',
    caption: 'New task. Name it and link the ticket.',
    icon: 'robot',
    label: 'New task',
  },
  {
    id: 'build-start-workflow',
    title: 'Feature workflow',
    alt: 'Kira Space Workflows tab showing the Feature workflow as a graph of steps.',
    caption: 'Workflow graph. Plan, implement, test and review as steps.',
    icon: 'robot',
    label: 'Feature workflow',
  },
  {
    id: 'build-start-step',
    title: 'Step editor',
    alt: 'Kira Space step editor with the agent, prompt and tool options of the Implement step.',
    caption: 'Step editor. Agent, prompt, tools and sandbox per step.',
    icon: 'robot',
    label: 'Step editor',
  },
  {
    id: 'build-start-launch',
    title: 'Launch menu',
    alt: 'Kira Space launch menu offering an agent session or a terminal in the task branch.',
    caption: 'Launch menu. Open an agent session or a terminal.',
    icon: 'robot',
    label: 'Launch menu',
  },
  {
    id: 'build-db-approval',
    title: 'Approvals dialog',
    alt: 'Kira Space approvals dialog over the agents window, with an agent write and a lifted limit waiting for an answer.',
    caption: 'Approvals. Agent writes wait for your answer.',
    icon: 'database',
    label: 'Approvals dialog',
  },
  {
    id: 'build-db-statement',
    title: 'Write approval',
    alt: 'Kira Space approval card with the connection, the write operation, the statement and the query plan evidence.',
    caption: 'Write approval. Connection, statement and plan evidence.',
    icon: 'database',
    label: 'Write approval',
  },
  {
    id: 'build-db-exemption',
    title: 'Limit exemption',
    alt: 'Kira Space approval card asking to lift an expensive query limit, with the measured rows and the agent reason.',
    caption: 'Limit exemption. Measured rows against the limit, with the reason.',
    icon: 'database',
    label: 'Limit exemption',
  },
  {
    id: 'build-db-audit',
    title: 'Audit trail',
    alt: 'Kira Space task details with the database audit trail listing each statement, its status and approval outcome.',
    caption: 'Audit trail. Every agent statement, masked, with its outcome.',
    icon: 'database',
    label: 'Audit trail',
  },
  {
    id: 'build-db-settings',
    title: 'Database MCP settings',
    alt: 'Kira Space Database MCP settings with switches for response preview, focus on approval and API requests.',
    caption: 'Settings. Preview responses, focus on approval, API requests.',
    icon: 'database',
    label: 'Database MCP settings',
  },
];
