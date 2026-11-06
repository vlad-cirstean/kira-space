import {
  type ProofView,
  proofDocumentPayloadSchema,
  proofKeyValuePayloadSchema,
  proofTablePayloadSchema,
} from '@shared/domain/proof';
import {
  createDocumentPageBuilder,
  createKeyValuePageBuilder,
  createTabularPageBuilder,
  type Page,
  unpagedPosition,
} from '@shared/protocol/page';
import { cleanupTabRuntime } from '@workbench/state/tabRuntime';
import { documentRow, drop, setPage } from '../views/console/resultPages';
import { useDocumentRowsStore } from '../views/shared/document/rows';

/** One result page per (session, revision, step); also the console-store tab id the grid keys on. */
export function proofPageKey(sessionId: string, rev: number, ordinal: number): string {
  return `proof:${sessionId}:${rev}:${ordinal}`;
}

function cellText(value: unknown): string | null {
  if (value === null || value === undefined) return null;
  if (typeof value === 'string') return value;
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/** Throws a ZodError when the payload does not match its view: Go validated it, but the window
 *  never trusts a payload it did not parse. */
export function buildProofPage(view: Exclude<ProofView, 'chart'>, data: unknown): Page {
  if (view === 'table') {
    const payload = proofTablePayloadSchema.parse(data);
    const builder = createTabularPageBuilder(
      payload.columns.map((c) => ({
        name: c.name,
        dataType: c.type,
        typeClass: c.type,
        nullable: true,
        isPrimaryKey: false,
        generated: false,
      })),
    );
    for (const row of payload.rows) builder.appendRow(row.map(cellText));
    return builder.finish(unpagedPosition(payload.rows.length));
  }
  if (view === 'document') {
    const payload = proofDocumentPayloadSchema.parse(data);
    const builder = createDocumentPageBuilder();
    for (const doc of payload.documents) builder.push(doc.id, JSON.stringify(doc.body));
    return builder.finish(unpagedPosition(payload.documents.length));
  }
  const payload = proofKeyValuePayloadSchema.parse(data);
  const builder = createKeyValuePageBuilder({ redisType: 'hash', ttlMs: null, memoryBytes: null });
  for (const entry of payload.entries) builder.push(entry.field, cellText(entry.value) ?? '');
  return builder.finish(unpagedPosition(payload.entries.length));
}

/** Registers the step's page for the result grid. Idempotent per key. */
export function mountProofPage(key: string, page: Page): void {
  setPage(key, page);
  if (page.kind === 'document') {
    useDocumentRowsStore().registerDocumentRows(key, (row) => documentRow(key, row));
  }
}

export function releaseProofPage(key: string): void {
  const rows = useDocumentRowsStore();
  drop(key);
  rows.unregisterDocumentRows(key);
  rows.dropRows(key);
  // Frees the step's console runtime record (expansion, widths) the grid created under this key.
  cleanupTabRuntime(key);
}
