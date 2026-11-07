import type { IconName } from './icons';
import type { ShotId } from './shotCapture';

export interface Feature {
  readonly title: string;
  /** Docs path relative to the site root, rendered as a link on the tile title. */
  readonly href: string;
  readonly color: 'sky' | 'pink' | 'lime' | 'violet' | 'amber' | 'teal';
  /** One punchy line shown on the tile. */
  readonly line: string;
  /** Real window shot shown on the tile. Without one the tile is text with `icon`. */
  readonly shot?: ShotId;
  readonly icon?: IconName;
}

export interface StoryChapter {
  readonly shot: string;
  readonly title: string;
  readonly points: readonly string[];
}

export interface AppContent {
  readonly name: string;
  readonly pageTitle: string;
  readonly description: string;
  readonly hero: string;
  readonly docs: string;
  readonly features: readonly Feature[];
}

// Facts come only from README.md and apps/kira-space/README.md.
export const SITE = {
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
  pageTitle: 'Kira Space: git, coding agents, databases and APIs for macOS',
  description:
    'Kira Space is a native macOS workbench: git client, code workspace, coding agents, a database client for ten engines, and an HTTP and gRPC client. Free and open source.',
  hero: 'space-agents-board',
  docs: 'docs/space/git/',
  features: [
    {
      title: 'Agents',
      shot: 'build-start-board',
      color: 'sky',
      line: 'Every task on a workflow. Every agent run in view.',
      href: 'docs/space/agents/',
    },
    {
      title: 'Studio: ten engines',
      shot: 'studio-connections',
      color: 'violet',
      line: 'PostgreSQL to S3. One tree of connections.',
      href: 'docs/studio/databases/',
    },
    {
      title: 'Data grid and SQL console',
      shot: 'studio-data-grid',
      color: 'amber',
      line: 'Write SQL, read EXPLAIN, stage edits.',
      href: 'docs/studio/data-grid/',
    },
    {
      title: 'Code workspace',
      shot: 'space-workspace',
      color: 'sky',
      line: 'File tree, Monaco, blame, search.',
      href: 'docs/space/workspace/',
    },
    {
      title: 'Runs',
      shot: 'space-runs',
      color: 'lime',
      line: 'Scripts on a schedule, run by Claude Code.',
      href: 'docs/shared/runs/',
    },
    {
      title: 'Docker',
      shot: 'studio-docker',
      color: 'teal',
      line: 'Containers, logs, exec. No context switch.',
      href: 'docs/studio/docker/',
    },
    {
      title: 'Git graph',
      icon: 'git-branch',
      color: 'teal',
      line: 'Branches, tags and diffs. Fast on any history.',
      href: 'docs/space/git/',
    },
    {
      title: 'Api module',
      icon: 'globe',
      color: 'pink',
      line: 'HTTP and gRPC. Paste a curl, get a request.',
      href: 'docs/studio/api-client/',
    },
    {
      title: 'Phone view',
      icon: 'device-mobile',
      color: 'teal',
      line: 'Your agents board on your phone, at home.',
      href: 'docs/space/phone/',
    },
    {
      title: 'Database MCP server',
      icon: 'plug',
      color: 'violet',
      line: 'Agent access to data: off until you say so.',
      href: 'docs/studio/database-mcp/',
    },
    {
      title: 'Keychain',
      icon: 'lock',
      color: 'pink',
      line: 'Credentials stay encrypted in the Keychain.',
      href: 'docs/reference/security/',
    },
  ],
};

export const STORY: readonly StoryChapter[] = [
  {
    shot: 'build-start-workflow',
    title: 'Plan it.',
    points: [
      'Pick a workflow for the task.',
      'Each step sets agent, prompt and tools.',
      'Start Claude Code or OpenCode from the task.',
    ],
  },
  {
    shot: 'space-git-graph',
    title: 'Read it.',
    points: [
      'Browse a virtualized commit log with branches and tags.',
      'Open diffs in the detail pane.',
      'Review a whole branch.',
    ],
  },
  {
    shot: 'studio-sql-console',
    title: 'Query it.',
    points: [
      'Run one statement or the whole script.',
      'Keep saved queries.',
      'Read EXPLAIN plans.',
    ],
  },
  {
    shot: 'studio-api',
    title: 'Call it.',
    points: [
      'Send HTTP and gRPC requests.',
      'Group requests in collections and environments.',
      'Import a curl command.',
    ],
  },
];

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
