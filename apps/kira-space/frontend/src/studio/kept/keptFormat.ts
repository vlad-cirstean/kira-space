import type { KeptSite, KeptUse } from '@shared/domain/kept';
import { formatClockTime, formatRelative } from '@theme/format';
import { t } from '@theme/i18n';
import type { ConsoleHoverInfo } from '../editor/hoverInfo';
import { keptRefs } from './keptHighlight';

interface TypedEntry {
  type: string;
  items: number;
}

/** "List of 12", "Text": what the variable holds, never its value. */
export function keptTypeLabel(e: TypedEntry): string {
  switch (e.type) {
    case 'array':
      return t('studio.kept.type.array.status', { count: e.items });
    case 'object':
      return t('studio.kept.type.object.status', { count: e.items });
    case 'string':
      return t('common.field.text.label');
    case 'number':
      return t('common.field.number.label');
    case 'boolean':
      return t('studio.kept.type.boolean.status');
    default:
      return t('studio.common.nullValue.label');
  }
}

interface DetailedEntry extends TypedEntry {
  source: { kind: string; request: string; client: string };
  createdAt: string;
  expiresAt: string;
}

/** The tooltip lines of a variable's name, empty ones skipped: who set it, the agent session, where the
 *  value may go (Settings), when it was set, when it expires, where a request reads it (the card), what
 *  it holds. */
export function keptDetails(
  e: DetailedEntry,
  extra: { origin?: string; usedIn?: string } = {},
): string[] {
  return [
    e.source.kind === 'user'
      ? t('studio.kept.supplied.status')
      : t('studio.kept.setBy.label', { request: e.source.request }),
    e.source.client,
    extra.origin ? t('studio.kept.origin.label', { origin: extra.origin }) : '',
    t('studio.kept.lastSet.label', { time: formatRelative(e.createdAt) }),
    t('studio.kept.expiresAt.status', { time: formatClockTime(new Date(e.expiresAt)) }),
    extra.usedIn ? t('studio.kept.usedIn.label', { sites: extra.usedIn }) : '',
    keptTypeLabel(e),
  ].filter((l) => l !== '');
}

/** Where a request reads a variable: "URL", "body", "header Authorization". `pairName` names the
 *  header, query or form entry at an index. */
export function keptSiteLabel(
  site: KeptSite,
  pairName: (field: 'header' | 'query' | 'form', index: number) => string,
): string {
  switch (site.field) {
    case 'url':
      return t('studio.http.url.label');
    case 'body':
      return t('common.body.label');
    case 'contentType':
      return t('studio.kept.site.contentType.label');
    case 'header':
      return t('studio.kept.site.header.label', { name: pairName('header', site.index) });
    case 'query':
      return t('studio.kept.site.query.label', { name: pairName('query', site.index) });
    case 'form':
      return t('studio.kept.site.form.label', { name: pairName('form', site.index) });
    case 'script':
      return site.part === 'transform'
        ? t('studio.dbMcp.approval.api.scriptTransform.label')
        : t('studio.dbMcp.approval.api.scriptPrepare.label');
    case 'statement':
      return t('studio.dbMcp.approval.detail.statement.label');
    case 'param':
      return t('common.field.parameterNamed.label', { name: String(site.index + 1) });
  }
}

function hoverOf(use: KeptUse, from: number, to: number): ConsoleHoverInfo {
  return {
    from,
    to,
    lines: [
      use.name,
      keptTypeLabel(use),
      t('studio.kept.expiresAt.status', { time: formatClockTime(new Date(use.expiresAt)) }),
    ],
    value: use.secret ? undefined : use.preview,
  };
}

/** Monaco hover for a reference in a body: the name, its type and expiry, the value preview. */
export function keptHover(uses: readonly KeptUse[]) {
  return (doc: string, offset: number): ConsoleHoverInfo | null => {
    const ref = keptRefs(doc).find((r) => offset >= r.from && offset < r.to);
    const use = ref && uses.find((u) => u.name === ref.name);
    return ref && use ? hoverOf(use, ref.from, ref.to) : null;
  };
}

/** The same hover for a placeholder of a SQL statement, found by the sites the backend sent. Only the
 *  statement as sent carries them: a formatted copy has moved offsets and gets none. */
export function keptStatementHover(uses: readonly KeptUse[], statement: string) {
  return (doc: string, offset: number): ConsoleHoverInfo | null => {
    if (doc !== statement) return null;
    for (const use of uses) {
      const site = use.sites.find(
        (s) => s.field === 'statement' && offset >= s.from && offset < s.to,
      );
      if (site) return hoverOf(use, site.from, site.to);
    }
    return null;
  };
}
