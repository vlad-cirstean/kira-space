import type { KiraSpaceApp } from '../fixtures';

interface ConnectionSpec {
  name: string;
  kind: string;
  host?: string | null;
  port?: number | null;
  database?: string | null;
  username?: string | null;
  password?: string | null;
}

// A connection is a file now and the new-connection dialog writes engine defaults only, so the
// real-backend specs create theirs through the bound call with the full config.
export async function createConnection(
  kira: Pick<KiraSpaceApp, 'call'>,
  spec: ConnectionSpec,
): Promise<void> {
  await kira.call('ConnectionsService', 'Create', {
    name: spec.name,
    kind: spec.kind,
    color: 'none',
    mode: 'fields',
    readOnly: false,
    host: spec.host ?? null,
    port: spec.port ?? null,
    database: spec.database ?? null,
    username: spec.username ?? null,
    password: spec.password ?? null,
    uri: null,
    options: {},
    preconnect: null,
    preconnectSidecar: false,
    preconnectDelaySec: 0,
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReviewResults: 'inherit',
  });
}
