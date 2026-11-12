import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { armFirstFrame, firstFrame, framesLate } from '../support/firstFrame';
import { IPC } from '../support/ipcChannels';
import { SESSION_ID, sessions } from './support/proof';

// The ready popup: first frame final, Open closes it before the window call answers. Backend half:
// proofflow TestProofWindow and TestProofEditFlow.

const dialog = '[data-testid="proof-ready-dialog"]';
const withTask = () => [{ ...sessions()[0], taskTitle: 'Fix checkout' }];

test('the first frame of the popup holds the task title and the proof title', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.proofSessions, response: withTask() }],
  });
  await armFirstFrame(page, 'proof-ready-dialog', 'proof-ready-proof');
  await emitPrompts(page, [routed('proof', SESSION_ID, { title: 'Proof ready: Checkout fix' })]);
  const r = await firstFrame(page);
  expect(framesLate(r)).toBe(0);
  expect(r.textAtDialogFrame).toBe('Checkout fix');
  await expect(page.locator(`${dialog} [data-slot="dialog-title"]`)).toHaveText('Fix checkout');
});

test('Open closes the popup before the window call answers', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.proofSessions, response: withTask() },
      {
        channel: IPC.proofOpenWindow,
        hold: true,
        response: contract('proof-window', 'ProofService.OpenWindow'),
      },
    ],
  });
  await emitPrompts(page, [routed('proof', SESSION_ID, { title: 'Proof ready: Checkout fix' })]);
  await page.locator('[data-testid="proof-ready-open"]').click();
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofOpenWindow))
    .toHaveLength(1);
  await expect(page.locator(dialog)).toHaveCount(0);
  control.release(IPC.proofOpenWindow);
});
