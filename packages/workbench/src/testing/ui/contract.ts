import { readFileSync } from 'node:fs';
import { join } from 'node:path';

/**
 * Reads a contract fixture the Go flow tests write and assert (internal/flowtest/contract.go) and
 * hydrates its placeholders into concrete values, so a UI spec answers its bridge mock with the
 * exact shape the real Go services return.
 */

const BASE_ISO = '2026-01-01T00:00:00.000Z';

const FIXED: Record<string, string> = {
  '<home>': '/home/test',
  '<work>': '/home/test/work',
  '<tmp>': '/tmp/test',
  '<space>': '/home/test/.kira-space',
  '<memory>': '/home/test/.kira-memory',
  '<bin>': '/home/test/bin',
  '<fake>': '/home/test/fake',
  '<masked>': '',
};

/** A contract fixture's untyped JSON object, for specs that spread and patch Go-written fixtures. */
// biome-ignore lint/suspicious/noExplicitAny: fixtures are Go-written JSON with no static shape
export type ContractJson = Record<string, any>;

export interface ContractOptions<T> {
  /** Parses the hydrated value (a zod schema from packages/shared/domain): TS type drift fails the spec too. */
  schema?: { parse(value: unknown): T };
  /** Replaces <http>, <https>, <grpc> with the spec's own fake origins. */
  origins?: { http?: string; https?: string; grpc?: string };
}

function hydrateString(s: string, origins: ContractOptions<unknown>['origins']): string {
  const map: Record<string, string> = { ...FIXED };
  if (origins?.http) map['<http>'] = origins.http;
  if (origins?.https) map['<https>'] = origins.https;
  if (origins?.grpc) map['<grpc>'] = origins.grpc;
  return s
    .replace(/<id:(\d+)>/g, (_, n: string) => `00000000-0000-4000-8000-${n.padStart(12, '0')}`)
    .replace(/<time>/g, BASE_ISO)
    .replace(/<[a-z]+>/g, (m) => map[m] ?? m);
}

function hydrate(v: unknown, origins: ContractOptions<unknown>['origins']): unknown {
  if (typeof v === 'string') return hydrateString(v, origins);
  if (Array.isArray(v)) return v.map((e) => hydrate(e, origins));
  if (v !== null && typeof v === 'object') {
    return Object.fromEntries(
      Object.entries(v).map(([k, e]) => [hydrateString(k, origins), hydrate(e, origins)]),
    );
  }
  return v;
}

const files = new Map<string, Record<string, unknown>>();

/** Binds the loader to one app's tests/contract directory. */
export function createContract(dir: string) {
  return function contract<T = unknown>(
    scenario: string,
    key: string,
    options: ContractOptions<T> = {},
  ): T {
    const path = join(dir, `${scenario}.json`);
    let doc = files.get(path);
    if (!doc) {
      doc = JSON.parse(readFileSync(path, 'utf8')) as Record<string, unknown>;
      files.set(path, doc);
    }
    if (!(key in doc)) {
      throw new Error(
        `contract ${scenario}.json has no key "${key}"; regenerate with KIRA_CONTRACT=write`,
      );
    }
    const value = hydrate(doc[key], options.origins);
    return options.schema ? options.schema.parse(value) : (value as T);
  };
}
