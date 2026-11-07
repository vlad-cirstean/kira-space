import type { IconName } from './icons';

export interface Feature {
  readonly icon: IconName;
  readonly title: string;
  readonly body: string;
  /** Docs path relative to the site root, rendered as a link on the card title. */
  readonly href?: string;
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
      href: 'docs/space/git/',
    },
    {
      icon: 'files',
      title: 'Code workspace',
      body: 'File tree, Monaco viewer, diffs, repository search, inline blame.',
      href: 'docs/space/workspace/',
    },
    {
      icon: 'robot',
      title: 'Agents',
      body: 'Task planner with workflows, agent runs and Claude Code or OpenCode sessions.',
      href: 'docs/space/agents/',
    },
    {
      icon: 'lightbulb',
      title: 'Memory',
      body: 'MCP server for Claude Code with keyword and semantic search.',
      href: 'docs/space/memory/',
    },
    {
      icon: 'sparkle',
      title: 'Runs',
      body: 'Scripts and smart scripts run by headless Claude Code, with cron schedules.',
      href: 'docs/shared/runs/',
    },
    {
      icon: 'device-mobile',
      title: 'Phone view',
      body: 'Serves the agents board to a phone on your home network.',
      href: 'docs/space/phone/',
    },
    {
      icon: 'database',
      title: 'Studio: ten engines',
      body: 'PostgreSQL, MariaDB, MySQL, SQLite, ClickHouse, MongoDB, Redis, Kafka, SQS, S3.',
      href: 'docs/studio/databases/',
    },
    {
      icon: 'table',
      title: 'Data grid and SQL console',
      body: 'Server-side filter and sort, staged SQL edits, saved queries, EXPLAIN plans.',
      href: 'docs/studio/data-grid/',
    },
    {
      icon: 'globe',
      title: 'Api module',
      body: 'HTTP and gRPC requests, collections, environments, curl import.',
      href: 'docs/studio/api-client/',
    },
    {
      icon: 'vm',
      title: 'Docker',
      body: 'Containers, images, volumes, networks, logs and exec terminals.',
      href: 'docs/studio/docker/',
    },
    {
      icon: 'plug',
      title: 'Database MCP server',
      body: 'Local, off by default, with per-connection permissions.',
      href: 'docs/studio/database-mcp/',
    },
    {
      icon: 'lock',
      title: 'Keychain',
      body: 'Credentials stay encrypted in the macOS Keychain.',
      href: 'docs/reference/security/',
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

export const BUILD_TEASER = {
  title: 'Build with Kira',
  body: 'Follow one task from plan to proof.',
  link: 'See the whole story',
} as const;

export const INSTALL = {
  command:
    'curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh | sh',
  steps: [
    'Run the command in Terminal. It installs a fresh copy or replaces an older one, then opens the app. An installed Kira Studio moves to the Trash.',
  ],
  note: 'Kira Space is beta. Builds are ad-hoc signed.',
  source: 'Build from source: see the README on GitHub.',
  macos: {
    title: 'Not notarized: what macOS asks',
    intro:
      'The build is ad-hoc signed, so macOS treats each build as a new app. Expect two prompts.',
    items: [
      {
        title: 'Keychain access',
        body: 'Kira Space asks to use its own key. It is the one item the app stores, and it encrypts your saved credentials. Choose Always Allow once per build. Kira Space never reads items of other apps.',
        file: 'keychain-access.webp',
        alt: 'macOS Keychain dialog asking to let Kira Space use its own key.',
        caption: 'Keychain. Choose Always Allow.',
      },
      {
        title: 'Folder access',
        body: 'macOS asks for access to folders that hold your repositories, such as Documents or Desktop. Allow it. Without access, Kira Space cannot read those repositories.',
        file: 'folder-access.webp',
        alt: 'macOS dialog asking to let Kira Space access a folder.',
        caption: 'Folders. Allow the ones with your repositories.',
      },
    ],
  },
} as const;
