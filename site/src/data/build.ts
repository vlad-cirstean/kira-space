import type { ShotId } from './shotCapture';

export interface BuildSection {
  readonly anchor: string;
  readonly step: number;
  readonly name: string;
  readonly pitch: string;
  readonly body: string;
  /** The first shot is the carousel slide and must be a window shot. */
  readonly shots: readonly ShotId[];
}

export const BUILD = {
  title: 'Build with Kira: plan, ask, branch, review, prove',
  heading: 'Build with Kira',
  intro:
    'One task, from plan to proof, in one app. Follow a single story: export orders as a CSV file.',
  description:
    'Plan a task on a workflow, let the agent ask your database with your approval, track branches and handover notes, review the change, and keep proof it works. Kira Space, step by step.',
  carouselLabel: 'Build with Kira in five steps',
} as const;

export const BUILD_SECTIONS: readonly BuildSection[] = [
  {
    anchor: 'start-task',
    step: 1,
    name: 'Plan a task on a workflow',
    pitch:
      'A task without a plan drifts. Kira puts every task on a workflow, so each stage has an owner and a prompt.',
    body: 'Capture the task on the plan board and pick a workflow. The Feature workflow runs Plan, Implement, Review and Proof. Each step sets the agent, its prompt, the database tools it may use and its sandbox. Start a Claude Code session, an OpenCode session or a terminal from the task.',
    shots: [
      'build-start-board',
      'build-start-capture',
      'build-start-workflow',
      'build-start-step',
      'build-start-launch',
    ],
  },
  {
    anchor: 'db-prompt',
    step: 2,
    name: 'Let the agent ask the database',
    pitch:
      'Agents need real data, and you need to know what they run. Kira lets the agent ask and shows you the statement first.',
    body: 'The database MCP server is off by default and set per connection. A write waits in the approvals dialog with its statement and query plan. An expensive query asks for a limit exemption with the measured cost. Every call lands in the task audit trail, with the statement masked.',
    shots: [
      'build-db-approval',
      'build-db-statement',
      'build-db-exemption',
      'build-db-audit',
      'build-db-settings',
    ],
  },
  {
    anchor: 'branches-handover',
    step: 3,
    name: 'Branches, handover and artifacts',
    pitch:
      'Work spreads over repositories and stages. Kira keeps the branches, the notes and the links of a task in one place.',
    body: 'Add one branch per repository to the task. Moving a task to the next stage opens a handover dialog with a note for the next agent. The Notes tab keeps the chain of handovers. The Details tab lists the pull request, the issue and the proof. A done task keeps them read-only.',
    shots: [
      'build-branches-panel',
      'build-branches-add',
      'build-handover-dialog',
      'build-handover-notes',
      'build-artifacts-list',
      'build-artifacts-done',
    ],
  },
  {
    anchor: 'review',
    step: 4,
    name: 'Review the change',
    pitch:
      'Agent changes need a careful read. Kira opens a review window for the branch and sends your comments back.',
    body: 'The review window shows the changed files, a side by side diff and the review agent. Comment on a line, ask a question or mark a thread addressed. Forward the open comments to the implementer step. Approve or request changes on the pull request from the same window.',
    shots: [
      'build-review-window',
      'build-review-thread',
      'build-review-agent',
      'build-review-forward',
      'build-review-actions',
    ],
  },
  {
    anchor: 'proof',
    step: 5,
    name: 'Prove it works',
    pitch:
      'A claim that it works needs evidence. The agent records proof, and you read it in a window of its own.',
    body: 'A proof session holds ordered steps: tables, charts, diagrams and key values from your databases. An agent edit to a proof waits for you, with a compare view. The proof joins the task Details tab, next to the handover notes.',
    shots: [
      'build-proof-window',
      'build-proof-chart',
      'build-proof-diagram',
      'build-proof-compare',
      'build-proof-chain',
    ],
  },
];
