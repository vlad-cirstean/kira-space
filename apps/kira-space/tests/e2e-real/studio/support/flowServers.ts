import { type ChildProcess, execFileSync, spawn } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { acquireBuildLock } from '@workbench/testing/e2eReal';

// The flow harness's real HTTP, HTTPS and gRPC servers (apps/kira-space/internal/flowharness/cmd/
// flowservers), built once under the shared build lock and spawned per test, so the browser tier
// and the Go flow tests share one server implementation.

const ROOT_DIR = resolve(__dirname, '../../../../../..');
const BINARY = resolve(ROOT_DIR, 'apps/kira-space/bin/flowservers');

export interface FlowServerRequest {
  method: string;
  path: string;
  query: string;
  host: string;
  proto: string;
  header: Record<string, string[]>;
  /** Base64, capped at 1 MiB; `bodyLen` and `bodySha256` cover all of it. */
  body: string | null;
  bodyLen: number;
  bodySha256: string;
  canceled: boolean;
  tls: boolean;
}

export interface FlowServers {
  /** `http://127.0.0.1:<port>` */
  http: string;
  /** `https://127.0.0.1:<port>`, certificate chained to `caFile` only. */
  https: string;
  /** `127.0.0.1:<port>`, plaintext gRPC with reflection. */
  grpc: string;
  /** `127.0.0.1:<port>`, TLS gRPC with reflection. */
  grpcTls: string;
  caFile: string;
  /** Holds `flow.proto` and its import, for loading the schema from a file. */
  protoDir: string;
  /** Every request the plain HTTP server received so far, in arrival order. */
  requests(): Promise<FlowServerRequest[]>;
}

let built: Promise<void> | undefined;

function buildBinary(): Promise<void> {
  built ??= (async () => {
    const release = await acquireBuildLock();
    try {
      await mkdir(resolve(ROOT_DIR, 'apps/kira-space/bin'), { recursive: true });
      execFileSync(
        'go',
        ['build', '-o', BINARY, './apps/kira-space/internal/studioflowharness/cmd/flowservers'],
        { cwd: ROOT_DIR, stdio: 'inherit' },
      );
    } finally {
      await release();
    }
  })();
  return built;
}

export async function startFlowServers(): Promise<{
  servers: FlowServers;
  stop: () => Promise<void>;
}> {
  await buildBinary();
  const proc: ChildProcess = spawn(BINARY, [], { stdio: ['pipe', 'pipe', 'inherit'] });
  const exited = new Promise<void>((r) => proc.once('exit', () => r()));
  const info = await new Promise<Omit<FlowServers, 'requests'>>((resolveInfo, reject) => {
    let buf = '';
    proc.once('exit', (code) =>
      reject(new Error(`flowservers exited (code ${code}) before ready`)),
    );
    proc.stdout?.on('data', (chunk: Buffer) => {
      buf += chunk.toString();
      const nl = buf.indexOf('\n');
      if (nl >= 0) resolveInfo(JSON.parse(buf.slice(0, nl)));
    });
  });
  const servers: FlowServers = {
    ...info,
    requests: async () =>
      (await (await fetch(`${info.http}/__requests`)).json()) as FlowServerRequest[],
  };
  return {
    servers,
    // Closing stdin makes flowservers exit cleanly; SIGKILL is the backstop.
    stop: async () => {
      proc.stdin?.end();
      const timer = setTimeout(() => proc.kill('SIGKILL'), 5_000);
      await exited;
      clearTimeout(timer);
    },
  };
}
