import type { EditorLanguageId } from '@shared/domain/editor';
import type { TypeClass } from '@shared/protocol/page';
import { detectFormat } from './detect';
import { beautifyFor, canBeautify, FORMAT_LANGUAGE } from './formats';

export interface ValueDiffSides {
  original: string;
  modified: string;
  language: EditorLanguageId;
}

/** Texts and language for a before/after value diff. A NULL side is empty. json and xml are
 *  indented only when every non-null side parses, so whitespace never fakes a change. */
export function valueDiffSides(
  before: string | null,
  after: string | null,
  column: { typeClass: TypeClass; dataType: string; name: string },
): ValueDiffSides {
  const format = detectFormat({
    text: after ?? before ?? '',
    typeClass: column.typeClass,
    dataType: column.dataType,
    columnName: column.name,
  })[0].format;
  let original = before ?? '';
  let modified = after ?? '';
  if (canBeautify(format)) {
    const sides = [before, after].map((text) =>
      text === null ? null : beautifyFor(format, text, 'indented'),
    );
    if (sides.every((r) => r === null || r.ok)) {
      original = sides[0]?.text ?? '';
      modified = sides[1]?.text ?? '';
    }
  }
  return { original, modified, language: FORMAT_LANGUAGE[format] };
}
