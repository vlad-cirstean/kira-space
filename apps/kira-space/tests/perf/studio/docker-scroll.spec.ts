import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import { studioTest as test } from '../../ui/fixtures';
import { container, openDocker, setupDocker } from '../../ui/studio/support/dockerFixture';

// Docker container list: NCONTAINERS (default 400), compose groups of 5 for the first 40%.

const NCONTAINERS = Number(process.env.NCONTAINERS ?? 400);
const ORIGINS = ['', 'devcontainer', 'testcontainers', 'kind'];
const GROUPED = Math.floor(NCONTAINERS * 0.4);
const list = Array.from({ length: NCONTAINERS }, (_, i) => {
  const state = i % 3 === 2 ? 'exited' : 'running';
  const id = `c-${String(i).padStart(4, '0')}`;
  const name = `svc-${String(i).padStart(4, '0')}`;
  if (i < GROUPED) {
    const project = `proj-${String(Math.floor(i / 5)).padStart(3, '0')}`;
    return container(id, name, state, {
      composeProject: project,
      composeService: `s${i % 5}`,
      origin: 'compose',
      originName: project,
    });
  }
  const origin = ORIGINS[i % ORIGINS.length] ?? '';
  return container(id, name, state, { origin, originName: origin ? 'x' : '' });
});

test('perf: docker container list', async ({ relaunch }, info) => {
  test.setTimeout(180_000);
  const { page } = await setupDocker(relaunch, { handlers: { Containers: () => list } });
  await openDocker(page);
  const box = await page.locator('[data-testid="docker-container-list"]').boundingBox();
  if (!box) throw new Error('container list has no box');
  await page.mouse.move(box.x + 60, box.y + 200);
  const mounted = await page
    .locator('[data-testid="docker-row"], [data-testid="docker-group"]')
    .count();
  console.log(`PERF docker rows mounted: ${mounted}`);

  const rss = new RssSampler();
  rss.start();
  const results: FlickResult[] = [];
  for (const { label, opts } of FLICK_LADDER) {
    results.push(await measureFlick(page, rss, label, opts));
  }
  rss.stop();
  await attachResults(info, results);
});
