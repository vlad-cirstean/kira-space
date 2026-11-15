import { expect, test } from '../ui/fixtures';
import { bandOf, openPlan } from '../ui/support/adeV2';

// P642: today fill, today bar, a normal day's bar and the gap between them.

test('plan day bars', async ({ relaunch }) => {
  const { window } = await openPlan(relaunch);
  await window.setViewportSize({ width: 1400, height: 1800 });
  const a = await bandOf(window, 0).boundingBox();
  const b = await bandOf(window, 1).boundingBox();
  if (!a || !b) throw new Error('bands missing');
  const clip = {
    x: a.x - 16,
    y: a.y,
    width: a.width + 16,
    height: b.y + b.height - a.y,
  };
  await expect(window).toHaveScreenshot('plan-day-bars.png', { clip });
});
