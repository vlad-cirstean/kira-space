import { lintSql } from '@shared/domain/sqlLint';
import { t } from '@theme/i18n';
import { scanJson, scanXml } from '../../../beautify';
import { BASE64_STD_RE, BASE64_URL_RE, base64ToStd, pickCsvShape } from './detect';
import type { CellFormat } from './formats';
import { parseTimestamp } from './timestamp';

// The format picker's own answer to "is my json broken, or my timestamp wrong" — every
// format validates for real, against the *effective* format (auto or override), on the same
// pure/synchronous 50 ms selection path detect.ts already runs on. An empty value is always
// valid for every format: there is nothing yet to be wrong.

export interface FormatProblem {
  message: string;
  offset?: number;
}

function validateJson(text: string): FormatProblem | null {
  const scan = scanJson(text);
  return scan.ok
    ? null
    : {
        message: t('studio.cellEditor.validate.json.error', { offset: scan.offset ?? 0 }),
        offset: scan.offset,
      };
}

function validateXml(text: string): FormatProblem | null {
  const scan = scanXml(text);
  return scan.ok ? null : { message: t('studio.cellEditor.validate.xml.error') };
}

function validateCsv(text: string): FormatProblem | null {
  const lines = text.split('\n').filter((l) => l.trim().length > 0);
  if (lines.length < 2) return { message: t('studio.cellEditor.validate.csvSingleLine.error') };
  return pickCsvShape(lines) ? null : { message: t('studio.cellEditor.validate.csvColumns.error') };
}

function validateSql(text: string): FormatProblem | null {
  const first = lintSql(text).find((i) => i.severity === 'error');
  return first ? { message: first.message, offset: first.from } : null;
}

function validateTimestamp(format: CellFormat, text: string): FormatProblem | null {
  return parseTimestamp(format, text)
    ? null
    : { message: t('studio.cellEditor.validate.timestamp.error') };
}

// Same shape check the detector itself runs (§5b), minus the score — a value that fails this is
// not decodable as this encoding at all, valid UTF-8 or not.
function validateBase64(text: string): FormatProblem | null {
  const isStd = BASE64_STD_RE.test(text);
  const isUrlSafe = !isStd && BASE64_URL_RE.test(text);
  if (!isStd && !isUrlSafe) return { message: t('studio.cellEditor.validate.base64.error') };
  try {
    atob(base64ToStd(text, isUrlSafe));
    return null;
  } catch {
    return { message: t('studio.cellEditor.validate.base64.error') };
  }
}

function validateHex(text: string): FormatProblem | null {
  const digits = text.startsWith('0x') || text.startsWith('0X') ? text.slice(2) : text;
  if (digits.length % 2 !== 0 || /[^0-9a-fA-F]/.test(digits)) {
    return { message: t('studio.cellEditor.validate.hex.error') };
  }
  return null;
}

// `truncated` short-circuits to null: the value on screen is a prefix the engine
// cut at MAX_CELL_BYTES, so "invalid at offset N" would be a statement about the truncation, not
// the data, and the user has no way to act on it — the rest was never fetched. The truncated chip
// (CellEditorView.vue's own `cell-editor-badge-truncated`) already says exactly what happened;
// this suppresses rather than softens the wording, since there is nothing honest a validator can
// say about a prefix ("valid so far" is not a claim any of these scanners make).
export function validateFormat(
  format: CellFormat,
  text: string,
  truncated?: boolean,
): FormatProblem | null {
  if (truncated) return null;
  const t = text.trim();
  if (t.length === 0) return null;
  const problem = ((): FormatProblem | null => {
    switch (format) {
      case 'json':
        return validateJson(t);
      case 'xml':
        return validateXml(t);
      case 'csv':
        return validateCsv(t);
      case 'sql':
        return validateSql(t);
      case 'iso8601':
      case 'epochSeconds':
      case 'epochMillis':
        return validateTimestamp(format, t);
      case 'base64':
        return validateBase64(t);
      case 'hex':
        return validateHex(t);
      case 'text':
        return null;
    }
  })();
  // P43 iter2 D30: `t` is trimmed, but the buffer CodeMirror shows is `text` verbatim — an
  // offset measured against `t` alone reports short by however much leading whitespace `trim()`
  // stripped (a value beginning with a newline and two spaces, say).
  if (problem?.offset === undefined) return problem;
  return { ...problem, offset: problem.offset + (text.length - text.trimStart().length) };
}
