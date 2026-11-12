import { expect, studioTest as test } from '../fixtures';
import { openSettings } from './support/settings';

// UI-only split of the Studio MCP settings, so no backend half: the nav lists one parent and four
// indented children, and each section shows only its own block.
const CHILDREN = ['connections', 'clusters', 'scripts', 'API'] as const;

const blocks: Record<string, string> = {
  'Studio MCP': 'settings-db-mcp-enabled',
  'Studio MCP connections': 'db-mcp-connections-empty',
  'Studio MCP clusters': 'db-mcp-kube-empty',
  'Studio MCP scripts': 'db-mcp-scripts-empty',
  'Studio MCP API': 'settings-db-mcp-api',
};

test('Studio MCP settings: nav shows the parent and four children, each section its own block', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch();
  await openSettings(page);
  const nav = (id: string) => page.locator(`[data-testid="settings-section-${id}"]`);

  await expect(nav('Studio MCP')).toHaveText('Studio MCP');
  await expect(nav('Database MCP')).toHaveCount(0);
  for (const [child, label] of [
    ['connections', 'Connections'],
    ['clusters', 'Kubernetes'],
    ['scripts', 'Scripts'],
    ['API', 'API tools'],
  ] as const) {
    await expect(nav(`Studio MCP ${child}`)).toHaveText(label);
    await expect(nav(`Studio MCP ${child}`)).toHaveClass(/pl-4/);
  }
  await expect(nav('Studio MCP')).not.toHaveClass(/pl-4/);

  for (const section of ['Studio MCP', ...CHILDREN.map((c) => `Studio MCP ${c}`)]) {
    await nav(section).click();
    for (const [other, testid] of Object.entries(blocks)) {
      const block = page.locator(`[data-testid="${testid}"]`);
      if (other === section) await expect(block).toBeVisible();
      else await expect(block).toBeHidden();
    }
  }
});
