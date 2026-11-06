import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openHttpMode } from './support/apiMode';
import { acceptConfirm } from './support/dialogs';
import { connectionRow } from './support/tree';

// P516: every secret field has an eye. It toggles what is typed, client-side; a saved value shows
// only where a gated reveal already exists (connections). No backend half: no new path, and
// connfiles_test.go / connection_form_test.go cover ConnectionsService.Reveal.

interface ApiView {
  files: unknown[];
  secrets: { name: string; set: boolean; referenced: boolean }[];
}
const apiView = contract<ApiView>('api-files', 'ApiConfigService.View');
const tree = contract<{ collections: { id: string }[] }>('api-files', 'CollectionsService.List');

test('api secret dialog: the eye shows and re-masks a typed value, nothing is revealed over IPC', async ({
  relaunch,
}) => {
  const view = { ...apiView, secrets: apiView.secrets.map((s) => ({ ...s, set: false })) };
  const after = { ...apiView, secrets: apiView.secrets.map((s) => ({ ...s, set: true })) };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.collectionsList, response: tree },
      { channel: IPC.variablesListEnvironments, response: [] },
      { channel: IPC.apiConfigView, response: view },
      { channel: IPC.apiConfigSetSecret, response: after },
    ],
  });
  await openHttpMode(page);
  await page.click('[data-testid="api-environments"]');
  const secret = page.locator(`[data-testid="api-secret-${view.secrets[0].name}"]`);
  await secret.locator('[data-testid="api-secret-set"]').click();
  const input = page.locator('[data-testid="api-secret-value"]');
  const eye = page.locator('[data-testid="api-secret-value-reveal"]');
  await input.fill('tok-123');
  await expect(input).toHaveAttribute('type', 'password');
  await eye.click();
  await expect(input).toHaveAttribute('type', 'text');
  await expect(eye).toHaveAttribute('aria-pressed', 'true');
  await eye.click();
  await expect(input).toHaveAttribute('type', 'password');
  await page.click('[data-testid="api-secret-save"]');
  await expect(page.locator('[data-testid="api-secret-dialog"]')).toHaveCount(0);
  await secret.locator('[data-testid="api-secret-set"]').click();
  await expect(input).toHaveValue('');
  await expect(input).toHaveAttribute('type', 'password');
  expect(
    control
      .log()
      .filter((e) => e.channel === IPC.connectionsReveal || e.channel === IPC.variablesReveal),
  ).toHaveLength(0);
});

test('api variables dialog: the eye of a saved secret row is off until a value is typed', async ({
  relaunch,
}) => {
  const KEY = 'api-variables-form';
  const ENV_FILE = 'staging.environment.yaml';
  const read = contract<{ file: string }>(KEY, 'file#read');
  const form = contract<{ text: string }>(KEY, 'form#read');
  const saved = {
    ...apiView,
    storage: { available: true, backend: 'keychain', insecureFallback: false, reason: null },
    secrets: [{ name: 'staging.token', set: true, referenced: true }],
  };
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.collectionsList,
        response: { collections: [], items: [] },
      },
      {
        channel: IPC.variablesListEnvironments,
        response: [
          {
            id: 'env-staging',
            name: 'Staging',
            sortOrder: 0,
            isActive: false,
            color: 'green',
            description: '',
            file: ENV_FILE,
          },
        ],
      },
      { channel: IPC.apiConfigFile, args: { file: ENV_FILE }, response: read },
      {
        channel: IPC.apiConfigForm,
        args: contract(KEY, 'args:ApiConfigService.Form#read'),
        response: form,
      },
      { channel: IPC.apiConfigView, response: saved },
    ],
  });
  await openHttpMode(page);
  await page.click('[data-testid="api-environments"]');
  await page.click('[data-testid="environment-edit"]');
  await page.locator('[data-testid="api-tab-variables"]').click();
  const input = page.locator('[data-testid="api-variable-secret-1"]');
  const eye = page.locator('[data-testid="api-variable-secret-1-reveal"]');
  await expect(input).toHaveValue('');
  await expect(eye).toBeDisabled();
  await eye.hover({ force: true });
  await expect(page.getByText('Saved values can’t be shown')).toBeVisible();
  await input.fill('typed');
  await expect(eye).toBeEnabled();
  await eye.click();
  await expect(input).toHaveAttribute('type', 'text');
});

const KEY = 'connection-form';
const FILE = 'local.yaml';
const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const ID = () => list()[0]?.id ?? '';
const detail = (over: Partial<ConnectionFileDetail> = {}) => ({
  ...contract<ConnectionFileDetail>(KEY, 'file#read'),
  file: FILE,
  id: ID(),
  ...over,
});
const formRead = (over: Record<string, unknown> = {}): ControlSnapshot => {
  const f = contract<{ text: string; problem: null; value: Record<string, unknown> }>(
    KEY,
    'form#read',
  );
  return {
    channel: IPC.connectionConfigForm,
    response: { ...f, value: { ...f.value, ...over } },
  };
};
const revealOk = (password: string | null, uri: string | null = null): ControlSnapshot => ({
  channel: IPC.connectionsReveal,
  args: { id: ID(), confirmed: false },
  response: { password, uri, error: null, outcome: 'revealed' },
});
const revealCalls = (control: { log(): { channel: string }[] }) =>
  control.log().filter((e) => e.channel === IPC.connectionsReveal);

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  extra: ControlSnapshot[],
  opts: { form?: ControlSnapshot; detail?: ConnectionFileDetail } = {},
) {
  const app = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: list() },
      { channel: IPC.connectionConfigView, response: view() },
      { channel: IPC.connectionConfigFile, response: opts.detail ?? detail() },
      opts.form ?? formRead(),
      ...extra,
    ],
  });
  await (await connectionRow(app.window, 'Local')).click({ button: 'right' });
  await app.window.click('[data-testid="menu-item-edit"]');
  await expect(app.window.locator('[data-testid="connection-file-dialog"]')).toBeVisible();
  await expect(app.window.locator('[data-testid="connection-name"]')).toHaveValue('Pg');
  return app;
}

test('connection secrets: the eye reveals the saved value once, a second press masks it', async ({
  relaunch,
}) => {
  const { window: page, control } = await open(relaunch, [revealOk('hunter2')]);
  const input = page.locator('[data-testid="connection-secret-value"]');
  const eye = page.locator('[data-testid="connection-secret-value-reveal"]');
  await expect(input).toHaveValue('');
  await expect(input).toHaveAttribute('type', 'password');
  expect(revealCalls(control)).toHaveLength(0);

  await eye.click();
  await expect(input).toHaveValue('hunter2');
  await expect(input).toHaveAttribute('type', 'text');
  await expect(input).toHaveAttribute('readonly', '');
  await expect(page.locator('[data-testid="connection-secret-save"]')).toBeDisabled();
  expect(revealCalls(control)).toHaveLength(1);

  await eye.click();
  await expect(input).toHaveValue('');
  await expect(input).toHaveAttribute('type', 'password');
  expect(revealCalls(control)).toHaveLength(1);
  expect(control.log().some((e) => e.channel === IPC.connectionConfigSetSecret)).toBe(false);
});

test('connection secrets: typing after hiding a reveal edits a fresh value', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, [revealOk('hunter2')]);
  const input = page.locator('[data-testid="connection-secret-value"]');
  const eye = page.locator('[data-testid="connection-secret-value-reveal"]');
  await eye.click();
  await expect(input).toHaveValue('hunter2');
  await eye.click();
  await input.fill('new-pass');
  await expect(input).toHaveValue('new-pass');
  await expect(input).not.toHaveAttribute('readonly', '');
  await expect(page.locator('[data-testid="connection-secret-save"]')).toBeEnabled();
});

test('connection general tab: the password reveals through its own eye', async ({ relaunch }) => {
  const { window: page, control } = await open(relaunch, [revealOk('hunter2')]);
  const input = page.locator('[data-testid="connection-password"]');
  const eye = page.locator('[data-testid="connection-password-reveal"]');
  await expect(input).toHaveValue('');
  await eye.click();
  await expect(input).toHaveValue('hunter2');
  await expect(input).toHaveAttribute('type', 'text');
  await eye.click();
  await expect(input).toHaveValue('');
  expect(revealCalls(control)).toHaveLength(1);
});

test('connection general tab: a URI reveal asks the URI confirmation when OS auth is unavailable', async ({
  relaunch,
}) => {
  const uriSecret = { kind: 'uri', name: 'pg.uri', referenced: true, set: true };
  const { window: page, control } = await open(
    relaunch,
    [
      {
        channel: IPC.connectionsReveal,
        args: { id: ID(), confirmed: false },
        response: { password: null, uri: null, error: null, outcome: 'confirmation-required' },
      },
      {
        channel: IPC.connectionsReveal,
        args: { id: ID(), confirmed: true },
        response: {
          password: null,
          uri: 'postgresql://u:hunter2@h:5432/db',
          error: null,
          outcome: 'revealed',
        },
      },
    ],
    {
      form: formRead({ mode: 'uri', uri: `\${secret:pg.uri}` }),
      detail: detail({ secrets: [uriSecret] as ConnectionFileDetail['secrets'] }),
    },
  );
  await page.locator('[data-testid="connection-uri-reveal"]').click();
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toBeVisible();
  await acceptConfirm(page);
  await expect(page.locator('[data-testid="connection-uri"]')).toHaveValue(
    'postgresql://u:hunter2@h:5432/db',
  );
  expect(revealCalls(control)).toHaveLength(2);
});

test('connection general tab: a reveal error shows and the field stays masked and empty', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, [
    {
      channel: IPC.connectionsReveal,
      args: { id: ID(), confirmed: false },
      response: { password: null, uri: null, error: 'Could not decrypt.', outcome: 'error' },
    },
  ]);
  const input = page.locator('[data-testid="connection-password"]');
  await page.locator('[data-testid="connection-password-reveal"]').click();
  await expect(page.locator('[data-testid="connection-secret-reveal-error"]')).toHaveText(
    'Could not decrypt.',
  );
  await expect(input).toHaveValue('');
  await expect(input).toHaveAttribute('type', 'password');
});

test('connection general tab: a file with no id cannot reveal a saved password', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, [], { detail: detail({ id: '' }) });
  await expect(page.locator('[data-testid="connection-password-reveal"]')).toBeDisabled();
});
