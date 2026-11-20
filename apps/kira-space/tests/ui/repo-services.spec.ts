import type { Page } from '@playwright/test';
import { codeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { openAdeRepos, type openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// The repo form lists the Run scripts that must run before work starts. Backend half: repoflow
// TestRepoServicesFile writes the repo-services contract.

interface FormFx {
  text: string;
  problem: { path: string; message: string } | null;
}
interface ScriptsFx {
  scripts: Record<string, unknown>[];
}

const t = (id: string) => `[data-testid="${id}"]`;
const KEY = 'repo-services';

const form = (name: string) => contract<FormFx>(KEY, `RepoConfigService.Form#${name}`);

// The picker contract's scripts, renamed to the ones the services name.
function scripts(): ScriptsFx {
  const list = contract<ScriptsFx>('repo-script-picker', 'scripts#list');
  const first = list.scripts.find((s) => s.kind === 'script');
  if (!first) throw new Error('no normal script in the picker contract');
  return {
    ...list,
    scripts: ['Dev DB', 'Dev server'].map((name, i) => ({
      ...first,
      id: `svc-${i}`,
      name,
      params: [],
      collection: '',
    })),
  };
}

const BASE = contract<{ text: string; file: string }>(KEY, 'args:RepoConfigService.Form#duplicate');

// One service picked: the first edit of either scenario. The contract answers the second.
const ONE_TEXT = `${BASE.text}services:\n  - script: Dev DB\n`;
const one: ControlSnapshot = {
  channel: IPC.repoConfigForm,
  args: {
    edits: [{ path: ['services'], unset: false, value: [{ script: 'Dev DB' }] }],
    file: BASE.file,
    text: BASE.text,
  },
  response: {
    problem: null,
    text: ONE_TEXT,
    value: { path: '<work>/proj', services: [{ script: 'Dev DB' }] },
  },
};

// The form sends the text it last got back, so the contract's args are matched on edits only.
const duplicate: ControlSnapshot = {
  channel: IPC.repoConfigForm,
  args: {
    edits: contract<{ edits: unknown }>(KEY, 'args:RepoConfigService.Form#duplicate').edits,
    file: BASE.file,
    text: ONE_TEXT,
  },
  response: form('duplicate'),
};

async function open(relaunch: Parameters<typeof openPlan>[0], extra: ControlSnapshot[] = []) {
  const app = await openAdeRepos(relaunch, [
    {
      channel: IPC.repoConfigFile,
      response: {
        editable: true,
        exists: true,
        file: 'proj.yaml',
        filePath: '/work/proj.yaml',
        hash: 'h1',
        lastGood: false,
        path: '<work>/proj',
        problem: null,
        text: BASE.text,
      },
    },
    { channel: IPC.customScriptsList, response: scripts() },
    {
      channel: IPC.repoConfigForm,
      args: { edits: [], file: BASE.file, text: BASE.text },
      response: { problem: null, text: BASE.text, value: { path: '<work>/proj' } },
    },
    one,
    ...extra,
  ]);
  await app.window.locator(t('repo-file')).waitFor();
  return app;
}

async function expectDraft(page: Page, text: string): Promise<void> {
  await page.locator(t('repo-file-tab-yaml')).click();
  await expect.poll(() => codeEditorText(page.locator(t('repo-file-text')))).toBe(text);
  await page.locator(t('repo-file-tab-form')).click();
}

test('adding a service through the picker edits the services list', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch);
  await expect(page.locator(t('repo-services-empty'))).toBeVisible();
  await page.locator(t('repo-add-service')).click();
  await page.locator(t('repo-service-run-script')).selectOption('Dev DB');
  await expect.poll(() => calls(control, IPC.repoConfigForm)).toHaveLength(2);
  await expectDraft(page, ONE_TEXT);
  await expect(page.locator(t('repo-service'))).toHaveCount(1);
});

test('contract: a duplicate service shows the problem under the section', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, [duplicate]);
  for (let i = 0; i < 2; i++) {
    await page.locator(t('repo-add-service')).click();
    await page.locator(t('repo-service-run-script')).last().selectOption('Dev DB');
    await expect(page.locator(t('repo-service-remove'))).toHaveCount(i + 1);
  }
  await expect(page.locator(t('repo-services-error'))).toContainText(
    form('duplicate').problem?.message ?? '',
  );
});
