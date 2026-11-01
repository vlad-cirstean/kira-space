import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Contract terminal. Backend half: termflow TestClaudeCodeLaunchWithoutClaude.
// Continue in terminal on a failed smart run opens a Claude Code tab; without the CLI the tab
// shows the install message in its failed state.

const DIR = {
  path: '/kira/automations/smart-1',
  mode: 'kira',
  base: '/kira',
  blocker: '',
  branch: '',
  pending: false,
};

const SMART = {
  id: 'smart-1',
  kind: 'smart',
  name: 'Ask Claude',
  command: 'Check the build.',
  params: [],
  smart: {
    model: 'sonnet',
    maxBudgetUsd: 1,
    timeout: '15m',
    tools: ['Read'],
    bashPatterns: [],
    mcp: [],
  },
  workingDir: '',
  dirMode: 'kira',
  useAdeDir: false,
  color: 'none',
  collectionId: null,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

const PREVIEW = {
  kind: 'smart',
  missing: [],
  blocker: '',
  dir: DIR,
  body: SMART.command,
  prompt: [{ text: SMART.command, var: '', value: '' }],
  suffix: 'When you are finished, call the finish_step tool.',
  env: [],
  command: '',
  model: 'sonnet',
  maxBudgetUsd: 1,
  timeout: '15m',
  tools: ['Read'],
  allowedTools: ['Read', 'mcp__kira-ade__finish_step'],
  mcpServers: [],
  hash: 'h1',
  needs: { tasks: [], branches: [] },
  ade: null,
};

const FAILED_RUN = {
  id: 'run-9',
  scriptId: SMART.id,
  scriptName: SMART.name,
  color: 'none',
  kind: 'smart',
  trigger: 'manual',
  state: 'failed',
  terminalId: '',
  cwd: DIR.path,
  command: '',
  outcome: {
    status: 'failed',
    reason: 'Stopped at the 1 USD budget',
    source: 'budget',
    reported: false,
  },
  createdAt: 1_000,
  startedAt: 1_000,
  finishedAt: 4_000,
  taskId: '',
  taskTitle: '',
  branchId: '',
  branchLabel: '',
  model: 'sonnet',
  sessionId: 'sess-1',
  prompt: SMART.command,
  params: [],
  tools: { tools: ['Read'], allowedTools: PREVIEW.allowedTools, mcpServers: [] },
};

test('contract: Continue in terminal without the claude CLI shows the install message', async ({
  relaunch,
}) => {
  const failure = contract<{ code: string; message: string }>(
    'terminal',
    'TerminalService.Open#claudeMissing',
  );
  expect(failure.message).toContain('Claude Code CLI not found');
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.customScriptsList, response: { collections: [], scripts: [SMART] } },
      { channel: IPC.scriptRunsPreview, response: PREVIEW },
      { channel: IPC.scriptRunsStart, response: { runId: 'run-9', terminal: null } },
      { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
      {
        channel: IPC.scriptRunsResumeCommand,
        response: { command: 'claude --resume sess-1', token: '' },
      },
      { channel: IPC.terminalOpen, error: failure },
    ],
  });
  await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
  await page.locator(`[data-testid="script-${SMART.id}"] [data-testid="script-play"]`).click();
  await page.locator('[data-testid="run-start"]').click();
  await emitWailsEvent(page, IPC.scriptRunsChanged, FAILED_RUN);
  await page.locator('[data-testid="run-continue"]').click();
  await expect(page.locator('[data-testid="repo-terminal-footer"]')).toContainText(failure.message);
});
