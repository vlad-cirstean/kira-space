import type { KeptSite, KeptUse } from '@shared/domain/kept';
import { formatBytes, formatClockTime } from '@theme/format';
import { t } from '@theme/i18n';
import type { ConsoleHoverInfo } from '../editor/hoverInfo';
import { keptRefs } from './keptHighlight';

interface TypedEntry {
  type: string;
  items: number;
  bytes: number;
}

/** "List of 12", "Text, 34 B": what the variable holds, never its value. */
export function keptTypeLabel(e: TypedEntry): string {
  switch (e.type) {
    case 'array':
      return t('studio.kept.type.array.status', { count: e.items });
    case 'object':
      return t('studio.kept.type.object.status', { count: e.items });
    case 'string':
      return t('studio.kept.type.string.status', { size: formatBytes(e.bytes) });
    case 'number':
      return t('common.field.number.label');
    case 'boolean':
      return t('studio.kept.type.boolean.status');
    default:
      return t('studio.common.nullValue.label');
  }
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
  }
}

/** Monaco hover for a reference in a body: the name, its type and expiry, the value preview. */
export function keptHover(uses: readonly KeptUse[]) {
  return (doc: string, offset: number): ConsoleHoverInfo | null => {
    const ref = keptRefs(doc).find((r) => offset >= r.from && offset < r.to);
    const use = ref && uses.find((u) => u.name === ref.name);
    if (!ref || !use) return null;
    return {
      from: ref.from,
      to: ref.to,
      lines: [
        use.name,
        keptTypeLabel(use),
        t('studio.kept.expiresAt.status', { time: formatClockTime(new Date(use.expiresAt)) }),
      ],
      value: use.secret ? undefined : use.preview,
    };
  };
}
