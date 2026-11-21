import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';
import { contract } from '../../support/contract';

// Approval items for the prompt specs: recorded contract items with the fields a case varies.

const pick = (file: string, key: string): ApprovalItem => {
  const v = contract<ApprovalItem | ApprovalItem[]>(file, key);
  const found = Array.isArray(v) ? v[0] : v;
  if (!found) throw new Error(`contract ${file} has no ${key}`);
  return found;
};

export const recorded = (file: string, key = 'ApprovalsService.List'): ApprovalItem =>
  liveItem(pick(file, key));

export const withPayload = (item: ApprovalItem, payload: Record<string, unknown>): ApprovalItem =>
  ({ ...item, payload: { ...(item.payload as object), ...payload } }) as ApprovalItem;

export const READ_SQL =
  "select o.id, o.created_at, c.email, sum(l.qty * l.price) as total from orders o join customers c on c.id = o.customer_id join order_lines l on l.order_id = o.id where o.created_at > now() - interval '30 days' group by o.id, o.created_at, c.email order by total desc limit 200";
export const WRITE_SQL =
  "update customers set marketing_opt_in = false, updated_at = now() where last_login < now() - interval '2 years' and country = 'DE'";

const cols = Array.from(
  { length: 80 },
  (_, i) => `  o.column_with_a_long_descriptive_name_${i}`,
).join(',\n');
export const LONG_SQL = `select\n${cols},\n  '${'x'.repeat(400)}' as unbroken_literal\nfrom orders o\nwhere o.created_at > now() - interval '30 days'\norder by o.id desc\nlimit 200`;

export const PLAN_OVER = {
  estimatedRowsRead: 1_240_000,
  thresholdRows: 100_000,
  overThreshold: true,
  issues: [
    { severity: 'warn', code: 'seq_scan', message: 'Seq scan on order_lines' },
    { severity: 'info', code: 'sort', message: 'Sort spills to disk' },
  ],
  issuesOmitted: 0,
};

/** A db.query item on the recorded dbmcp contract. */
export function dbItem(over: {
  cls: 'read' | 'write' | 'ddl';
  sql: string;
  readOnly?: boolean;
  enforcement?: string;
  plan?: typeof PLAN_OVER | null;
  name?: string;
  color?: string;
  kind?: string;
  location?: string;
}): ApprovalItem {
  const b = recorded('dbmcp');
  const tone = over.cls === 'read' ? 'neutral' : over.cls === 'write' ? 'caution' : 'danger';
  return {
    ...b,
    target: {
      ...b.target,
      name: over.name ?? 'shop prod',
      kind: over.kind ?? 'postgres',
      location: over.location ?? 'db.internal · shop / public',
      readOnly: over.readOnly ?? false,
      color: over.color ?? 'red',
    },
    origin: { ...b.origin, cwd: '~/shop', task: 'Churn report' },
    operation: { class: over.cls, tone },
    payload: {
      ...(b.payload as object),
      statement: over.sql,
      mode: over.cls,
      detected: over.cls,
      enforcement: over.enforcement ?? '',
      reviewOffered: true,
      plan: over.plan ?? null,
    },
  } as ApprovalItem;
}

/** A db.exemption item: the agent asks to lift the row limit, with its reason. */
export function exemptionItem(over: { sql: string; plan?: typeof PLAN_OVER | null }): ApprovalItem {
  const ex = recorded('approvals-exemption');
  return withPayload(
    {
      ...ex,
      target: {
        ...ex.target,
        name: 'shop prod',
        kind: 'postgres',
        color: 'red',
        location: 'db.internal · shop / public',
      },
    } as ApprovalItem,
    {
      statement: over.sql,
      enforcement: 'session',
      reason: 'The monthly churn report needs every order of the last 30 days.',
      limits: [{ limit: 'expensive_query', measured: 1_240_000, max: 100_000, unit: 'rows' }],
      plan: over.plan ?? null,
    },
  );
}

/** An api.request item with five headers (two masked) and four variables (two secret). */
export function richApi(key: string): ApprovalItem {
  const it = recorded('approvals-api', key);
  const p = it.payload as Record<string, unknown>;
  return {
    ...it,
    payload: {
      ...p,
      url: String(p.url).replace('/echo', '/v2/tenants/{{tenant}}/orders'),
      headers: [
        { name: 'Authorization', value: 'Bearer {{token}}', masked: true },
        { name: 'Content-Type', value: 'application/json', masked: false },
        { name: 'X-Tenant', value: '{{tenant}}', masked: false },
        { name: 'Idempotency-Key', value: '7f3c2a10-5b1e-4f7a-9d2e-1a6b0c9e4d21', masked: false },
        { name: 'X-Api-Key', value: '{{partner_key}}', masked: true },
      ],
      variables: [
        { name: 'base_url', value: 'https://api.shop.example', secret: false },
        { name: 'tenant', value: 'acme-de', secret: false },
        { name: 'token', value: '••••', secret: true },
        { name: 'partner_key', value: '••••', secret: true },
      ],
      credentials: [
        { name: 'token', hosts: ['api.shop.example'] },
        { name: 'partner_key', hosts: ['api.shop.example'] },
      ],
    },
  } as ApprovalItem;
}

export const HEX =
  '7b227573657273223a5b7b226964223a393030373139393235343734303939332c226e616d65223a22416461227d2c7b226964223a322c226e616d65223a22426f62227d5d2c22746f74616c223a327d';

/** A GET whose query has four pairs, one of them a 150-character hex value. */
export function longQueryItem(): ApprovalItem {
  const it = richApi('item#json');
  const p = it.payload as Record<string, unknown>;
  return {
    ...it,
    payload: {
      ...p,
      method: 'GET',
      url: `http://127.0.0.1:8080/v2/tenants/{{tenant}}/raw?ct=application%2Fjson&hex=${HEX}&page=2&token={{token}}&sort=-created_at`,
      body: null,
    },
  } as ApprovalItem;
}

const DIALOG_BODY = '[data-testid="approvals-dialog"] [data-slot="dialog-body"]';

/** Scrolls the dialog body to its end and returns what the scroll proves. */
export async function scrollBodyToEnd(page: Page): Promise<{
  scrollable: boolean;
  scrollTop: number;
}> {
  return page.locator(DIALOG_BODY).evaluate((el) => {
    const scrollable = el.scrollHeight > el.clientHeight;
    el.scrollTop = el.scrollHeight;
    return { scrollable, scrollTop: el.scrollTop };
  });
}

/** How far text pokes past the right edge of each Monaco editor in the dialog: wrapped text is 0. */
export function editorOverflows(page: Page): Promise<number[]> {
  return page.locator('[data-testid="approvals-dialog"] .monaco-editor').evaluateAll((els) =>
    els.map((el) => {
      const right = el.getBoundingClientRect().right;
      const ends = [...el.querySelectorAll('.view-line span')].map(
        (s) => s.getBoundingClientRect().right,
      );
      return Math.max(0, ...ends) - right;
    }),
  );
}
