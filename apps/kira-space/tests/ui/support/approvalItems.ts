import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';

const ORIGIN = { client: 'Claude Code 2.1.318', cwd: '~/shop', task: '', step: '', source: '' };

function base(id: string, kind: string, name: string, kindLabel: string, payload: unknown) {
  return liveItem({
    id,
    kind,
    createdAt: 0,
    expiresAt: 0,
    origin: ORIGIN,
    target: { name, kind: kindLabel, color: '', readOnly: false, location: '' },
    operation: { class: 'read', tone: 'neutral' },
    reason: 'permission',
    payload,
  } as ApprovalItem);
}

/** A db.query approval whose statement is `lines` long. */
export function dbQueryItem(id: string, lines: number): ApprovalItem {
  const statement = Array.from({ length: lines }, (_, i) => `select ${i + 1} from shop_t${i}`).join(
    '\n',
  );
  return base(id, 'db.query', 'shop db', 'postgres', {
    connectionId: 'c1',
    enforcement: 'session',
    reviewResult: false,
    statement,
    mode: 'read',
    detected: 'read',
    plan: null,
    reviewOffered: false,
  });
}

/** An api.request approval with a JSON body of `lines` lines. */
export function apiRequestItem(id: string, lines: number): ApprovalItem {
  const body = JSON.stringify(
    Object.fromEntries(Array.from({ length: lines - 2 }, (_, i) => [`k${i}`, i])),
    null,
    2,
  );
  return base(id, 'api.request', 'api.staging.acme.dev', 'http', {
    method: 'POST',
    url: 'https://api.staging.acme.dev/v1/orders',
    host: 'api.staging.acme.dev',
    environment: { id: 'e1', name: 'Staging', color: 'amber' },
    query: [],
    headers: [{ name: 'Content-Type', value: 'application/json', masked: false }],
    auth: null,
    body: { contentType: 'application/json', text: body, truncated: false, form: null },
    variables: [],
    credentials: [],
    followRedirects: false,
    timeoutSeconds: 30,
  });
}
