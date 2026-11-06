import type { IconName } from './icons';

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
  readonly name: string;
  readonly icon: IconName;
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
  title: 'Kira Space',
  tagline: 'One native macOS app. Git and coding agents. Databases and APIs.',
  availability: 'Free and open source. macOS 26+ on Apple Silicon.',
} as const;

export const APP: AppContent = {
  name: 'Kira Space',
  icon: 'git-branch',
  pageTitle: 'Kira Space: git, coding agents, databases and APIs for macOS',
  description:
    'Kira Space is a native macOS workbench: git client, code workspace, coding agents, memory, a database client for ten engines, and an HTTP and gRPC client. Free and open source.',
  summary:
    'Git client and code workspace with coding agents, memory and a terminal. Database, API, Docker and Kubernetes modules in the same window.',
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
      icon: 'sparkle',
      title: 'Runs',
      body: 'Scripts and smart scripts run by headless Claude Code, with cron schedules.',
    },
    {
      icon: 'device-mobile',
      title: 'Phone view',
      body: 'Serves the agents board to a phone on your home network.',
    },
    {
      icon: 'database',
      title: 'Studio: ten engines',
      body: 'PostgreSQL, MariaDB, MySQL, SQLite, ClickHouse, MongoDB, Redis, Kafka, SQS, S3.',
    },
    {
      icon: 'table',
      title: 'Data grid and SQL console',
      body: 'Server-side filter and sort, staged SQL edits, saved queries, EXPLAIN plans.',
    },
    {
      icon: 'globe',
      title: 'Api module',
      body: 'HTTP and gRPC requests, collections, environments, curl import.',
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
};

export const INSTALL = {
  command:
    'curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh | sh',
  steps: [
    'Run the command in Terminal. It installs a fresh copy or replaces an older one, then opens the app. An installed Kira Studio moves to the Trash.',
    'On first launch after an update, the Keychain may ask to use confidential information. Choose Always Allow once for that build.',
  ],
  note: 'Kira Space is beta. Builds are ad-hoc signed, not notarized.',
  source: 'Build from source: see the README on GitHub.',
} as const;
