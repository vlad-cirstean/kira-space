import type { IconName } from './icons';

export interface Feature {
  readonly icon: IconName;
  readonly title: string;
  readonly body: string;
  /** Docs path relative to the site root, rendered as a link on the card title. */
  readonly href?: string;
  /** Bento tile: mini UI key, accent colour token and grid span. */
  readonly visual: string;
  readonly color: 'sky' | 'pink' | 'lime' | 'violet' | 'amber' | 'teal';
  readonly span: 'hero' | 'wide' | 'half' | 'third';
  /** One punchy line shown on the tile. */
  readonly line: string;
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

export const ENGINES = [
  'PostgreSQL',
  'MariaDB',
  'MySQL',
  'SQLite',
  'ClickHouse',
  'MongoDB',
  'Redis',
  'Kafka',
  'SQS',
  'S3',
] as const;

export const ALSO = ['HTTP', 'gRPC', 'Docker', 'Kubernetes', 'Claude Code', 'OpenCode'] as const;

export const HERO = {
  eyebrow: 'Kira Space',
  line1: 'Git, agents and data.',
  line2: 'One native window.',
  lead: 'Plan with coding agents, read history, query ten database engines and call APIs. Kira Space keeps it in one Mac app.',
  command: 'curl -fsSL …/install.sh | sh',
} as const;

export const APP: AppContent = {
  name: 'Kira Space',
  icon: 'git-branch',
  pageTitle: 'Kira Space: git, coding agents, databases and APIs for macOS',
  description:
    'Kira Space is a native macOS workbench: git client, code workspace, coding agents, a database client for ten engines, and an HTTP and gRPC client. Free and open source.',
  summary:
    'Git client and code workspace with coding agents and a terminal. Database, API, Docker and Kubernetes modules in the same window.',
  hero: 'space-agents-board',
  docs: 'docs/space/git/',
  features: [
    {
      icon: 'git-branch',
      title: 'Git graph',
      visual: 'graph',
      color: 'teal',
      span: 'half',
      line: 'Branches, tags and diffs. Fast on any history.',
      body: 'Virtualized commit log, detail pane, diffs, search and branch review.',
      href: 'docs/space/git/',
    },
    {
      icon: 'files',
      title: 'Code workspace',
      visual: 'workspace',
      color: 'sky',
      span: 'third',
      line: 'File tree, Monaco, blame, search.',
      body: 'File tree, Monaco viewer, diffs, repository search, inline blame.',
      href: 'docs/space/workspace/',
    },
    {
      icon: 'robot',
      title: 'Agents',
      visual: 'agents',
      color: 'sky',
      span: 'hero',
      line: 'Every task on a workflow. Every agent run in view.',
      body: 'Task planner with workflows, agent runs and Claude Code or OpenCode sessions.',
      href: 'docs/space/agents/',
    },
    {
      icon: 'sparkle',
      title: 'Runs',
      visual: 'runs',
      color: 'lime',
      span: 'third',
      line: 'Scripts on a schedule, run by Claude Code.',
      body: 'Scripts and smart scripts run by headless Claude Code, with cron schedules.',
      href: 'docs/shared/runs/',
    },
    {
      icon: 'device-mobile',
      title: 'Phone view',
      visual: 'phone',
      color: 'teal',
      span: 'third',
      line: 'Your agents board on your phone, at home.',
      body: 'Serves the agents board to a phone on your home network.',
      href: 'docs/space/phone/',
    },
    {
      icon: 'database',
      title: 'Studio: ten engines',
      visual: 'engines',
      color: 'violet',
      span: 'half',
      line: 'PostgreSQL to S3. One grid.',
      body: 'PostgreSQL, MariaDB, MySQL, SQLite, ClickHouse, MongoDB, Redis, Kafka, SQS, S3.',
      href: 'docs/studio/databases/',
    },
    {
      icon: 'table',
      title: 'Data grid and SQL console',
      visual: 'sql',
      color: 'amber',
      span: 'wide',
      line: 'Write SQL, read EXPLAIN, stage edits.',
      body: 'Server-side filter and sort, staged SQL edits, saved queries, EXPLAIN plans.',
      href: 'docs/studio/data-grid/',
    },
    {
      icon: 'globe',
      title: 'Api module',
      visual: 'api',
      color: 'pink',
      span: 'wide',
      line: 'HTTP and gRPC. Paste a curl, get a request.',
      body: 'HTTP and gRPC requests, collections, environments, curl import.',
      href: 'docs/studio/api-client/',
    },
    {
      icon: 'vm',
      title: 'Docker',
      visual: 'docker',
      color: 'sky',
      span: 'third',
      line: 'Containers, logs, exec. No context switch.',
      body: 'Containers, images, volumes, networks, logs and exec terminals.',
      href: 'docs/studio/docker/',
    },
    {
      icon: 'plug',
      title: 'Database MCP server',
      visual: 'mcp',
      color: 'violet',
      span: 'third',
      line: 'Agent access to data: off until you say so.',
      body: 'Local, off by default, with per-connection permissions.',
      href: 'docs/studio/database-mcp/',
    },
    {
      icon: 'lock',
      title: 'Keychain',
      visual: 'keychain',
      color: 'pink',
      span: 'third',
      line: 'Credentials stay encrypted in the Keychain.',
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
    'curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-space/main/scripts/install.sh | sh',
  steps: [
    'Run the command in Terminal. It installs a fresh copy or replaces an older one, then opens the app.',
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
        image: {
          file: 'keychain-access.webp',
          alt: 'macOS Keychain dialog asking to let Kira Space use its own key.',
          caption: 'Keychain. Choose Always Allow.',
        },
      },
      {
        title: 'Folder access',
        body: 'macOS asks for access to folders that hold your repositories, such as Documents or Desktop. Allow it. Without access, Kira Space cannot read those repositories.',
      },
    ],
  },
} as const;
