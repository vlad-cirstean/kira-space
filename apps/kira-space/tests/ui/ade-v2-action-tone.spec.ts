import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { reviewLast, sessionsOf, stageSession, taskAtStep } from './support/adeStage';
import { openPlan, openTask } from './support/adeV2';
import type { ControlSnapshot } from './support/types';

// Step action tone outline (P692): Start agent, Finish green, in the plan card, the task panel
// header and the workflow block. Frontend only; no IPC change, so no flow half. Expected colours
// come from the live tokens, so a second theme needs no edit here.

const t = (id: string) => `[data-testid="${id}"]`;

function token(page: Page, name: string): Promise<string> {
  return page.evaluate((n) => {
    const span = document.createElement('span');
    span.style.color = `var(${n})`;
    document.body.appendChild(span);
    const color = getComputedStyle(span).color;
    span.remove();
    return color;
  }, name);
}

const look = (host: Locator) =>
  host.evaluate((el) => {
    const cs = getComputedStyle(el);
    return { border: cs.borderTopColor, text: cs.color };
  });

function hosts(page: Page, kind: string) {
  const action = t(`ade-task-action-${kind}`);
  return {
    card: page.locator(`${t('ade-task')}[data-task-id="T_auth"] ${t('ade-card-head')} ${action}`),
    panel: page.locator(`${t('ade-panel-actions')} ${action}`),
    block: page.locator(
      `${t('ade-workflow-block')} ${t('ade-step-block')}[data-state="now"] ${action}`,
    ),
  };
}

const cases: { name: string; kind: string; tokenName: string; snapshots: ControlSnapshot[] }[] = [
  {
    name: 'Start is the agent outline in every host',
    kind: 'session',
    tokenName: '--kira-agent',
    snapshots: [
      taskAtStep('T_auth', 'review'),
      sessionsOf('T_auth', [stageSession('st1', 'T_auth', 'spec')]),
    ],
  },
  {
    name: 'Finish is the green outline in every host',
    kind: 'finish',
    tokenName: '--kira-status-green',
    snapshots: [
      taskAtStep('T_auth', 'review'),
      reviewLast(),
      sessionsOf('T_auth', [stageSession('st1', 'T_auth', 'review')]),
    ],
  },
];

for (const c of cases) {
  test(c.name, async ({ relaunch }) => {
    const { window: page } = await openPlan(relaunch, c.snapshots);
    await openTask(page, 'T_auth');
    const want = await token(page, c.tokenName);
    expect(want).not.toBe(await token(page, '--kira-border'));
    const h = hosts(page, c.kind);
    // The plan card icon is toolbar: tone on the glyph, no border.
    await expect(h.card).toBeVisible();
    await expect.poll(() => look(h.card)).toEqual({ border: 'rgba(0, 0, 0, 0)', text: want });
    for (const host of [h.panel, h.block]) {
      await expect(host).toBeVisible();
      await expect.poll(() => look(host)).toEqual({ border: want, text: want });
    }
    await page.keyboard.press('Tab');
    await h.panel.focus();
    await expect
      .poll(() =>
        h.panel.evaluate((el) => ({
          visible: el.matches(':focus-visible'),
          color: getComputedStyle(el).outlineColor,
        })),
      )
      .toEqual({ visible: true, color: want });
  });
}
