import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';

// Guards the shared focus ring (packages/theme/src/base.css) against animating on focus —
// see docs/HISTORY.md Before the fix, `outline-color` (and, on
// `transition-all` elements, `outline-width`/`outline-offset`) interpolated from `currentcolor`/
// `medium`/`0px` over Tailwind's default 150ms transition, so a freshly-focused element showed a
// white-to-blue (or thick-to-thin) flash instead of the ring's resting state on the first frame.

async function resolveFocusColor(page: import('@playwright/test').Page): Promise<string> {
  return page.evaluate(() => {
    const span = document.createElement('span');
    span.style.color = 'var(--kira-focus)';
    document.body.appendChild(span);
    const color = getComputedStyle(span).color;
    span.remove();
    return color;
  });
}

// Real, trusted Tab keypresses, not a script-invoked el.focus() — WebKit only matches
// `:focus-visible` on a button/toggle when focus arrives from an actual keyboard event; a
// programmatic focus() never does, regardless of what was focused just before (confirmed live
// against this dialog: the standalone repro page had no prior pointer interaction to
// set that modality, which this real click-driven dialog does). Capped and asserted so a future
// tab-order change fails loudly instead of silently sampling the wrong element.
async function tabUntilTestId(
  page: import('@playwright/test').Page,
  testid: string,
): Promise<void> {
  const cap = 30;
  for (let i = 0; i < cap; i++) {
    await page.keyboard.press('Tab');
    const current = await page.evaluate(() => document.activeElement?.getAttribute('data-testid'));
    if (current === testid) return;
  }
  throw new Error(`tabUntilTestId: never reached [data-testid="${testid}"] within ${cap} tabs`);
}

test('new connection dialog Input and Button focus rings never animate', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await page.click('[data-testid="add-connection"]');
  await expect(page.locator('[data-testid="new-connection-dialog"]')).toBeVisible();
  await page.click('[data-testid="connection-kind-postgres"]');
  // Create must be enabled (reachable in tab order) for the Button assertion below.
  await page.fill('[data-testid="new-connection-name"]', 'Focus Ring');

  const focusColor = await resolveFocusColor(page);

  // Input: the dialog auto-focuses this field, so it's blurred first — a no-op focus() would
  // start no transition and false-pass the pre-fix run.
  const input = page.locator('[data-testid="new-connection-name"]');
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await expect.poll(() => input.evaluate((el) => el.getAnimations().length)).toBe(0);
  const inputSample = await input.evaluate((el) => {
    (el as HTMLElement).focus();
    const outlineTransitions: string[] = [];
    for (const animation of el.getAnimations()) {
      const prop = (animation as CSSTransition).transitionProperty;
      if (typeof prop === 'string' && prop.startsWith('outline')) outlineTransitions.push(prop);
    }
    return {
      focusVisible: el.matches(':focus-visible'),
      outlineColor: getComputedStyle(el).outlineColor,
      outlineTransitions,
    };
  });
  expect(inputSample.focusVisible).toBe(true);
  expect(inputSample.outlineColor).toBe(focusColor);
  expect(inputSample.outlineTransitions).toEqual([]);

  // Button.
  await tabUntilTestId(page, 'new-connection-create');
  const buttonSample = await page.evaluate(() => {
    const el = document.activeElement as HTMLElement;
    const outlineTransitions: string[] = [];
    for (const animation of el.getAnimations()) {
      const prop = (animation as CSSTransition).transitionProperty;
      if (typeof prop === 'string' && prop.startsWith('outline')) outlineTransitions.push(prop);
    }
    return {
      focusVisible: el.matches(':focus-visible'),
      outlineColor: getComputedStyle(el).outlineColor,
      outlineTransitions,
    };
  });
  expect(buttonSample.focusVisible).toBe(true);
  expect(buttonSample.outlineColor).toBe(focusColor);
  expect(buttonSample.outlineTransitions).toEqual([]);
});
