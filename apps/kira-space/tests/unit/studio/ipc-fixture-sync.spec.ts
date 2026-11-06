import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

const KINDS = ['clickhouse', 'kafka', 'mariadb', 'mysql', 'redis', 'sqs'] as const;

// The Go backend asserts testdata/*.json; the frontend specs mock from tests/ipc/*.fixture.ts.
// Both come from one KIRA_IPC_FIXTURES=write capture; this fails when they diverge.
describe('IPC fixtures: Go JSON copy matches the TS module', () => {
  for (const kind of KINDS) {
    test(kind, async () => {
      const mod = await import(`../../ipc/${kind}/${kind}.fixture`);
      const json = JSON.parse(
        readFileSync(
          new URL(`../../../internal/ipcfixture/testdata/${kind}.fixture.json`, import.meta.url),
          'utf8',
        ),
      );
      expect(JSON.parse(JSON.stringify(mod.controlSnapshots))).toEqual(json.controlSnapshots);
      expect(JSON.parse(JSON.stringify(mod.portSnapshots))).toEqual(json.portSnapshots);
    });
  }
});
