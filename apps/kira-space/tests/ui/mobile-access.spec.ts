import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P212: the Mobile access settings pane and the phone pairing prompt.

const lan = {
  interface: 'en0',
  address: '192.168.1.20',
  subnet: '192.168.1.0/24',
  routerIp: '192.168.1.1',
  routerMac: 'aa:bb:cc:dd:ee:ff',
};
const stoppedStatus = {
  enabled: false,
  running: false,
  port: 7790,
  appUrl: '',
  agentInput: false,
  error: '',
  stopReason: '',
  stopDetail: '',
  current: lan,
  trusted: null,
  trustedAt: 0,
};
const untrustedStatus = {
  ...stoppedStatus,
  enabled: true,
  stopReason: 'notTrusted',
  stopDetail: 'No trusted network. Trust this network to start the phone server.',
};
const runningStatus = {
  ...stoppedStatus,
  enabled: true,
  running: true,
  appUrl: 'http://192.168.1.20:7790/',
  trusted: { ...lan, address: '' },
  trustedAt: 1_700_000_000_000,
};
const awayStatus = {
  ...runningStatus,
  running: false,
  appUrl: '',
  current: null,
  stopReason: 'away',
  stopDetail: 'Stopped: this computer is not on the trusted network 192.168.1.0/24.',
};
const device = {
  id: 'dev-1',
  label: 'Pixel 7',
  userAgent: 'Mozilla/5.0',
  createdAt: 1_700_000_000_000,
  lastSeenAt: 1_700_000_100_000,
  lastIp: '192.168.1.40',
  revokedAt: null,
  expiresAt: Date.now() + 5 * 24 * 3_600_000 + 3_600_000,
  canWrite: true,
  canAgentInput: false,
};

async function openPane(window: import('@playwright/test').Page): Promise<void> {
  await emitWailsEvent(window, IPC.openSettings, undefined);
  await window.locator('[data-testid="settings-section-Mobile access"]').click();
}

test('the plaintext warning shows whether the server is off or on', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.mobileStatusGet, response: stoppedStatus }],
  });
  await openPane(window);
  const warning = window.locator('[data-testid="mobile-access-plaintext-warning"]');
  await expect(warning).toContainText('plain HTTP');
  await emitWailsEvent(window, IPC.mobileStatus, runningStatus);
  await expect(warning).toBeVisible();
});

test('enabled without a trusted network says why and shows no QR', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.mobileStatusGet, response: untrustedStatus }],
  });
  await openPane(window);
  await expect(window.locator('[data-testid="mobile-access-stopped-reason"]')).toContainText(
    'Trust this network',
  );
  await expect(window.locator('[data-testid="mobile-access-trusted"]')).toHaveText(
    'No trusted network',
  );
  await expect(window.locator('[data-testid="mobile-access-step-app"]')).toHaveCount(0);
});

test('trusting the network confirms first, then shows the QR and URL', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: untrustedStatus },
      { channel: IPC.mobileTrustNetwork, response: runningStatus },
    ],
  });
  await openPane(window);
  await window.locator('[data-testid="mobile-access-trust"]').click();
  await expect(window.locator('[data-testid="confirm-dialog"]')).toContainText(
    '192.168.1.0/24 (router aa:bb:cc:dd:ee:ff)',
  );
  expect(control.log().some((e) => e.channel === IPC.mobileTrustNetwork)).toBe(false);
  await window.locator('[data-testid="confirm-dialog-confirm"]').click();

  const app = window.locator('[data-testid="mobile-access-step-app"]');
  await expect(app).toContainText('http://192.168.1.20:7790/');
  await expect(app.locator('svg')).toHaveCount(1);
  await expect(window.locator('[data-testid="mobile-access-trusted"]')).toContainText(
    '192.168.1.0/24, router 192.168.1.1 (aa:bb:cc:dd:ee:ff)',
  );
});

test('a status event with reason away shows it and hides the QR', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.mobileStatusGet, response: runningStatus }],
  });
  await openPane(window);
  await expect(window.locator('[data-testid="mobile-access-step-app"]')).toBeVisible();

  await emitWailsEvent(window, IPC.mobileStatus, awayStatus);
  await expect(window.locator('[data-testid="mobile-access-stopped-reason"]')).toContainText(
    'not on the trusted network 192.168.1.0/24',
  );
  await expect(window.locator('[data-testid="mobile-access-step-app"]')).toHaveCount(0);
});

test('forgetting the network confirms first, then calls ForgetNetwork', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: runningStatus },
      { channel: IPC.mobileForgetNetwork, response: untrustedStatus },
    ],
  });
  await openPane(window);
  await window.locator('[data-testid="mobile-access-forget"]').click();
  await window.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect
    .poll(() => control.log().some((e) => e.channel === IPC.mobileForgetNetwork))
    .toBe(true);
  await expect(window.locator('[data-testid="mobile-access-trusted"]')).toHaveText(
    'No trusted network',
  );
});

test('applying a new port calls SetPort', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: runningStatus },
      { channel: IPC.mobileSetPort, response: { ...runningStatus, port: 7800 } },
    ],
  });
  await openPane(window);
  await window.locator('[data-testid="mobile-access-port"]').fill('7800');
  await window.locator('[data-testid="mobile-access-apply-port"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.mobileSetPort)?.args)
    .toEqual({ port: 7800 });
});

test('a device row shows its expiry and an expired device is hidden', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: runningStatus },
      {
        channel: IPC.mobileDevicesList,
        response: [device, { ...device, id: 'dev-2', label: 'Old', expiresAt: Date.now() - 1000 }],
      },
    ],
  });
  await openPane(window);
  await expect(window.locator('[data-testid="mobile-device-expires-dev-1"]')).toContainText(
    'Access expires in 5d',
  );
  await expect(window.locator('[data-testid="mobile-device-row-dev-2"]')).toHaveCount(0);
});

test('revoking a phone confirms first, then calls Revoke', async ({ relaunch }) => {
  const { window, control } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: runningStatus },
      { channel: IPC.mobileDevicesList, response: [device] },
      { channel: IPC.mobileRevoke, response: null },
    ],
  });
  await openPane(window);
  await expect(window.locator('[data-testid="mobile-device-row-dev-1"]')).toContainText('Pixel 7');

  await window.locator('[data-testid="mobile-device-revoke-dev-1"]').click();
  await expect(window.locator('[data-testid="confirm-dialog"]')).toBeVisible();
  expect(control.log().some((e) => e.channel === IPC.mobileRevoke)).toBe(false);
  await window.locator('[data-testid="confirm-dialog-confirm"]').click();

  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.mobileRevoke)?.args)
    .toEqual({ id: 'dev-1' });
});

test('permission switches call the bound methods; the global switch gates agent input', async ({
  relaunch,
}) => {
  const { window, control } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: runningStatus },
      { channel: IPC.mobileDevicesList, response: [device] },
      { channel: IPC.mobileSetDevicePermissions, response: null },
      { channel: IPC.mobileSetAgentInput, response: { ...runningStatus, agentInput: true } },
    ],
  });
  await openPane(window);
  const agentInput = window.locator('[data-testid="mobile-device-agent-input-dev-1"]');
  await expect(agentInput).toBeDisabled();

  await window.locator('[data-testid="mobile-device-write-dev-1"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.mobileSetDevicePermissions)?.args)
    .toEqual({ id: 'dev-1', write: false, agentInput: false });

  await window.locator('[data-testid="mobile-access-agent-input"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.mobileSetAgentInput)?.args)
    .toEqual({ enabled: true });
  await expect(agentInput).toBeEnabled();
});

test('a device row says how many terminals the phone controls', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [
      { channel: IPC.mobileStatusGet, response: runningStatus },
      { channel: IPC.mobileDevicesList, response: [device] },
    ],
  });
  await openPane(window);
  await expect(window.locator('[data-testid="mobile-device-terminals-dev-1"]')).toHaveCount(0);
  await emitWailsEvent(window, IPC.mobileTerminals, [
    {
      terminalId: 't1',
      sessionId: 's1',
      deviceId: 'dev-1',
      label: 'Pixel 7',
      connected: true,
      since: 0,
      returnsAt: 0,
    },
  ]);
  await expect(window.locator('[data-testid="mobile-device-terminals-dev-1"]')).toHaveText(
    'Controlling 1 terminal',
  );
});

test('a pushed device list replaces the rendered one', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.mobileStatusGet, response: runningStatus }],
  });
  await openPane(window);
  await expect(window.locator('[data-testid="mobile-devices-empty"]')).toBeVisible();
  await emitWailsEvent(window, IPC.mobileDevices, [device]);
  await expect(window.locator('[data-testid="mobile-device-row-dev-1"]')).toBeVisible();
});

const { pending: request } = contract<{ pending: Record<string, unknown> & { requestId: string } }>(
  'mobile-pairing',
  'event:kira:mobile:pairing#request',
);
request.expiresAtMs = Date.now() + 120_000;

test('contract: the pairing prompt shows the code and origin; approve and deny call the bound methods', async ({
  relaunch,
}) => {
  const { window, control } = await relaunch({
    control: [
      { channel: IPC.mobilePairingApprove, response: { result: 'resolved' } },
      { channel: IPC.mobilePairingDeny, response: { result: 'resolved' } },
    ],
  });
  const dialog = window.locator('[data-testid="mobile-pairing-dialog"]');
  await expect(dialog).toHaveCount(0);

  await emitWailsEvent(window, IPC.mobilePairing, { pending: request, queued: 1 });
  await emitPrompts(window, [routed('mobile-pairing', request.requestId)]);
  await expect(dialog).toBeVisible();
  await expect(window.locator('[data-testid="mobile-pairing-code"]')).toHaveText('4821');
  await expect(window.locator('[data-testid="mobile-pairing-origin"]')).toContainText('127.0.0.1');
  await expect(window.locator('[data-testid="mobile-pairing-expires"]')).toHaveText(
    /^Expires in [01]:\d\d$/,
  );

  await window.locator('[data-testid="mobile-pairing-approve"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.mobilePairingApprove)?.args)
    .toEqual({ id: request.requestId });

  await emitWailsEvent(window, IPC.mobilePairing, {
    pending: { ...request, requestId: 'req-2' },
    queued: 1,
  });
  await emitPrompts(window, [routed('mobile-pairing', 'req-2')]);
  await window.locator('[data-testid="mobile-pairing-deny"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.mobilePairingDeny)?.args)
    .toEqual({ id: 'req-2' });
});
