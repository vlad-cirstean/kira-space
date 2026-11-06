import type { EditorLanguageId } from '@shared/domain/editor';
import { type MessageKey, t } from '@theme/i18n';
import {
  type BeautifyMode,
  type BeautifyResult,
  beautifyJson,
  beautifyXml,
} from '../../../beautify';

/** The closed vocabulary, decided once. uuid/url dropped (both were inert on selection,
 *  changing nothing but a label and, for uuid, whether one button was enabled);
 *  reordered common-first into three groups the picker separates:
 *  read-it-directly formats, then time, then binary encodings. */
export const CELL_FORMATS = [
  'text',
  'json',
  'xml',
  'csv',
  'sql',
  'iso8601',
  'epochSeconds',
  'epochMillis',
  'base64',
  'hex',
] as const;
export type CellFormat = (typeof CELL_FORMATS)[number];

const FORMAT_LABEL_KEY: Record<CellFormat, MessageKey> = {
  text: 'common.plainText.label',
  json: 'studio.copy.format.json.label',
  xml: 'studio.cellEditor.format.xml.label',
  csv: 'studio.copy.format.csv.label',
  sql: 'studio.cellEditor.format.sql.label',
  iso8601: 'studio.cellEditor.format.iso8601.label',
  epochSeconds: 'studio.cellEditor.format.epochSeconds.label',
  epochMillis: 'studio.cellEditor.format.epochMillis.label',
  base64: 'studio.cellEditor.format.base64.label',
  hex: 'studio.cellEditor.format.hex.label',
};

/** The key stays `iso8601` (a stored override value and a Playwright attribute); only the label is wording. */
export function formatLabel(format: CellFormat): string {
  return t(FORMAT_LABEL_KEY[format]);
}

export function formatAutoLabel(format: CellFormat): string {
  return t('studio.cellEditor.format.auto.label', { format: formatLabel(format) });
}

/** One sentence per format: the picker's rows (hover) and its trigger's tooltip read the same
 *  text. Distinct from typeGlossary.ts, which describes column types. */
export function formatHelp(format: CellFormat): string {
  return t(`studio.cellEditor.format.${format}.description`);
}

/** P42 D27: where one group ends and the next begins in the picker — read-it-directly formats,
 *  then time, then binary encodings that open a decoded-text pane. */
export const FORMAT_GROUPS: readonly (readonly CellFormat[])[] = [
  ['text', 'json', 'xml', 'csv', 'sql'],
  ['iso8601', 'epochSeconds', 'epochMillis'],
  ['base64', 'hex'],
];

/** Which Monaco grammar renders a format; `plain` means no language extension. */
export const FORMAT_LANGUAGE: Record<CellFormat, EditorLanguageId> = {
  text: 'plain',
  json: 'json',
  xml: 'xml',
  csv: 'plain',
  sql: 'sql',
  iso8601: 'plain',
  epochSeconds: 'plain',
  epochMillis: 'plain',
  base64: 'plain',
  hex: 'plain',
};

/** True only where a lossless reformatter exists (D11): json and xml. */
export function canBeautify(format: CellFormat): boolean {
  return format === 'json' || format === 'xml';
}

/**
 * Applied to whatever `text` the caller passes in — the current edit buffer, so
 * hand-editing then beautifying formats the edit instead of discarding it. Reversibility
 * (indented <-> compact) comes from both modes being lossless, not from always starting over at
 * the stored value. Offered for `json` and `xml` only; every other format has no lossless
 * formatter and the caller must not invoke this for one (see `canBeautify` above).
 */
export function beautifyFor(format: CellFormat, text: string, mode: BeautifyMode): BeautifyResult {
  if (format === 'json') return beautifyJson(text, mode);
  if (format === 'xml') return beautifyXml(text, mode);
  return {
    text,
    ok: false,
    reason: { key: 'studio.beautify.noFormatter.error', params: { format } },
  };
}
