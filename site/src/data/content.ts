import type { IconName } from './icons';
import type { AppId } from './shots';

export interface Feature {
  readonly icon: IconName;
  readonly title: string;
  readonly body: string;
}

export interface Highlight {
  readonly shot: string;
  readonly title: string;
  readonly points: readonly string[];
}

export interface AppContent {
  readonly id: AppId;
  readonly name: string;
  readonly icon: IconName;
  readonly hint: string;
  readonly pageTitle: string;
  readonly description: string;
  readonly summary: string;
  readonly hero: string;
  readonly docs: string;
  readonly features: readonly Feature[];
  readonly highlights: readonly Highlight[];
}

// Facts come only from README.md and apps/kira-space/README.md.
export const SITE = {
  title: 'Kira Studio and Kira Space',
  tagline: 'Two native macOS apps. Databases and APIs. Git and coding agents.',
  availability: 'Free and open source. macOS 26+ on Apple Silicon.',
} as const;

export const APPS: readonly AppContent[] = [
  {
    id: 'studio',
    name: 'Kira Studio',
    icon: 'database',
    hint: 'Databases and APIs',
    pageTitle: 'Kira Studio: database and API client for macOS',
    description:
      'Kira Studio is a native macOS database client for ten engines and an HTTP and gRPC client. Free and open source.',
    summary: 'Database client for ten engines. HTTP and gRPC client. One button switches modes.',
    hero: 'studio-data-grid',
    docs: 'docs/studio/databases/',
    features: [
      {
        icon: 'database',
        title: 'Ten engines',
        body: 'PostgreSQL, MariaDB, MySQL, SQLite, ClickHouse, MongoDB, Redis, Kafka, SQS, S3.',
      },
      {
        icon: 'table',
        title: 'Data grid',
        body: 'Virtualized, with server-side filter and sort and staged SQL edits.',
      },
      {
        icon: 'code',
        title: 'SQL console',
        body: 'Run a statement or all, saved queries, EXPLAIN plans.',
      },
      {
        icon: 'globe',
        title: 'Api mode',
        body: 'HTTP and gRPC requests, collections, environments, curl import.',
      },
      {
        icon: 'sparkle',
        title: 'Runs',
        body: 'Scripts and smart scripts run by headless Claude Code.',
      },
      {
        icon: 'vm',
        title: 'Docker',
        body: 'Containers, images, volumes, networks, logs and exec terminals.',
      },
      {
        icon: 'plug',
        title: 'Database MCP server',
        body: 'Local, off by default, with per-connection permissions.',
      },
      {
        icon: 'lock',
        title: 'Keychain',
        body: 'Credentials stay encrypted in the macOS Keychain.',
      },
    ],
    highlights: [
      {
        shot: 'studio-sql-console',
        title: 'Write SQL, read the result',
        points: [
          'Run one statement or the whole script.',
          'Keep saved queries.',
          'Read EXPLAIN plans.',
        ],
      },
      {
        shot: 'studio-api',
        title: 'Call an API without leaving the app',
        points: [
          'Send HTTP and gRPC requests.',
          'Group requests in collections and environments.',
          'Import a curl command.',
        ],
      },
    ],
  },
  {
    id: 'space',
    name: 'Kira Space',
    icon: 'git-branch',
    hint: 'Git and coding agents',
    pageTitle: 'Kira Space: git client with coding agents for macOS',
    description:
      'Kira Space is a native macOS git client and code workspace with coding agents, memory and a terminal. Free and open source.',
    summary: 'Git client and code workspace with coding agents, memory and a terminal.',
    hero: 'space-agents-board',
    docs: 'docs/space/git/',
    features: [
      {
        icon: 'git-branch',
        title: 'Git graph',
        body: 'Virtualized commit log, detail pane, diffs, search and branch review.',
      },
      {
        icon: 'files',
        title: 'Code workspace',
        body: 'File tree, Monaco viewer, diffs, repository search, inline blame.',
      },
      {
        icon: 'robot',
        title: 'Agents',
        body: 'Task planner with workflows, agent runs and Claude Code or OpenCode sessions.',
      },
      {
        icon: 'lightbulb',
        title: 'Memory',
        body: 'MCP server for Claude Code with keyword and semantic search.',
      },
      {
        icon: 'terminal',
        title: 'Runs and terminal',
        body: 'Scripts with run history and cron schedules.',
      },
      {
        icon: 'device-mobile',
        title: 'Phone view',
        body: 'Serves the agents board to a phone on your home network.',
      },
    ],
    highlights: [
      {
        shot: 'space-git-graph',
        title: 'Read history at a glance',
        points: [
          'Browse a virtualized commit log with branches and tags.',
          'Open diffs in the detail pane.',
          'Review a whole branch.',
        ],
      },
      {
        shot: 'space-memory',
        title: 'Memory that shows its source',
        points: [
          'Every fact shows its origin and scope.',
          'Search by keyword or by meaning.',
          'Claude Code reads it through an MCP server.',
        ],
      },
    ],
  },
];

export const INSTALL = {
  command: (app: AppId) =>
    `curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh | sh -s -- --app=${app}`,
  steps: [
    'Run the command in Terminal. It installs a fresh copy or replaces an older one, then opens the app.',
    'On first launch after an update, the Keychain may ask to use confidential information. Choose Always Allow once for that build.',
  ],
  note: 'Both apps are beta. Builds are ad-hoc signed, not notarized.',
  source: 'Build from source: see the README on GitHub.',
} as const;
