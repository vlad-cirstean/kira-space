import { beautifyJson, beautifyXml, scanJson } from '../../../frontend/src/studio/beautify';
import { findRanges } from '../../../frontend/src/studio/editor/findRanges';
import { detectFormat } from '../../../frontend/src/studio/views/shared/celleditor/detect';
import { beautifyFor } from '../../../frontend/src/studio/views/shared/celleditor/formats';
import { validateFormat } from '../../../frontend/src/studio/views/shared/celleditor/validate';
import { rowsToJson } from '../../../frontend/src/studio/views/shared/clipboardFormats';
import {
  beautifyShellText,
  parseDocument,
  toPlainJson,
  toRelaxedText,
  toShellText,
  tryParseShellText,
} from '../../../frontend/src/studio/views/shared/document/ejson';

// P163 probe: pure caller functions, timed inside WebKit (the production engine). Built to an
// IIFE by parse-callers.spec.ts and injected into a page; returns median ms over `reps` runs.

const time = (reps: number, fn: () => unknown): number => {
  fn();
  const xs: number[] = [];
  for (let i = 0; i < reps; i++) {
    const t = performance.now();
    fn();
    xs.push(performance.now() - t);
  }
  xs.sort((a, b) => a - b);
  return xs[Math.floor(xs.length / 2)] ?? 0;
};

const jsonOf = (bytes: number): string => {
  const rows: string[] = [];
  let len = 2;
  for (let n = 0; len < bytes; n++) {
    const r = `{"id":${n},"name":"user-${n}","email":"user-${n}@example.com","active":true,"score":12.5,"tags":["a","b"]}`;
    rows.push(r);
    len += r.length + 1;
  }
  return `[${rows.join(',\n')}]`;
};
const xmlOf = (bytes: number): string => {
  const rows: string[] = [];
  let len = 13;
  for (let n = 0; len < bytes; n++) {
    const r = `<item id="${n}"><name>user-${n}</name><v>${n}</v><tag>a</tag></item>`;
    rows.push(r);
    len += r.length;
  }
  return `<root>\n${rows.join('\n')}\n</root>`;
};
const docOf = (bytes: number): string =>
  `{"_id":{"$oid":"000000000000000000000001"},"name":"w","createdAt":{"$date":{"$numberLong":"1704067200000"}},"n":{"$numberInt":"5"},"pad":"${'x'.repeat(bytes)}"}`;
const shellScriptOf = (bytes: number): string => {
  const items: string[] = [];
  let len = 2;
  for (let n = 0; len < bytes; n++) {
    const l = `{ _id: ObjectId("${n.toString(16).padStart(24, '0')}"), name: "w${n}", at: ISODate("2024-01-01T00:00:00Z") }`;
    items.push(l);
    len += l.length + 2;
  }
  return `[${items.join(',\n')}]`;
};

const KB64 = 64 * 1024;
const MB = 1024 * 1024;

export function runAll(reps: number): Record<string, number> {
  const out: Record<string, number> = {};
  const r = (k: string, fn: () => unknown, n = reps) => {
    out[k] = Math.round(time(n, fn) * 10) / 10;
  };
  const j5 = jsonOf(5 * MB);
  const j256 = jsonOf(256 * 1024);
  const x256 = xmlOf(256 * 1024);
  const j64 = jsonOf(KB64);
  const x64 = xmlOf(KB64);
  // 2: response find, 5 MB / 12 MB text, query hitting many times
  r('2.findRanges 5MB', () => findRanges(j5, 'user-1', 0), 3);
  r('2.findRanges 12MB', () => findRanges(jsonOf(12 * MB), 'user-1', 0), 3);
  // 6: response compare, both sides 256 KiB
  const detect = (b: string) => {
    const json = beautifyJson(b, 'indented');
    if (json.ok) return json;
    return beautifyXml(b, 'indented');
  };
  r('6.detectAndBeautify json 256KB', () => detect(j256));
  r('6.detectAndBeautify xml 256KB', () => detect(x256));
  // 7: cell editor 64 KiB
  r('7.detectFormat json 64KB', () =>
    detectFormat({ text: j64, typeClass: 'text', dataType: 'text', columnName: 'c' }),
  );
  r('7.detectFormat xml 64KB', () =>
    detectFormat({ text: x64, typeClass: 'text', dataType: 'text', columnName: 'c' }),
  );
  r('7.validateFormat json 64KB', () => validateFormat('json', j64));
  r('7.validateFormat xml 64KB', () => validateFormat('xml', x64));
  r('7.scanJson 64KB', () => scanJson(j64));
  r('7.beautifyFor json 64KB', () => beautifyFor('json', j64, 'indented'));
  r('7.beautifyFor xml 64KB', () => beautifyFor('xml', x64, 'indented'));
  // 8: byteLabel
  r('8.TextEncoder 64KB', () => new TextEncoder().encode(j64).length);
  // 9: documents rows
  const d64 = docOf(KB64);
  r('9.parseDocument 64KB', () => parseDocument(d64));
  const d400 = docOf(200);
  r('9.parseDocument x60 400B', () => {
    for (let i = 0; i < 60; i++) parseDocument(d400);
  });
  // 10: one fieldNamesOnPage pass (JSON.parse + Object.keys per body), page of 10 000 x 400 B / 2 000 x 8 KB
  const fieldPass = (bodies: string[]) => () => {
    const names = new Set<string>();
    for (const b of bodies) for (const k of Object.keys(JSON.parse(b))) names.add(k);
    return names;
  };
  const small = Array.from({ length: 10_000 }, () => docOf(150));
  const big = Array.from({ length: 2000 }, () => docOf(8192));
  r('10.fieldNames pass 10000x400B', fieldPass(small), 3);
  r('10.fieldNames pass 2000x8KB', fieldPass(big), 3);
  // 11: row menu, one 64 KiB body
  r('11.toPlainJson 64KB', () => toPlainJson(d64));
  r('11.toRelaxedText 64KB', () => toRelaxedText(d64));
  r('11.toShellText 64KB', () => toShellText(d64));
  r('11.beautifyJson 64KB', () => beautifyJson(d64, 'indented'));
  // 14: console Mongo lint, scan of a large script
  const s64 = shellScriptOf(KB64);
  const s1m = shellScriptOf(MB);
  r('14.tryParseShellText 64KB', () => tryParseShellText(s64));
  r('14.tryParseShellText 1MB', () => tryParseShellText(s1m), 3);
  r('14.beautifyShellText 1MB', () => beautifyShellText(s1m, 'indented'), 3);
  // 15: EXPLAIN plan JSON.parse
  const plan = jsonOf(MB);
  r('15.JSON.parse 1MB', () => JSON.parse(plan));
  // 16: grid copy as JSON, 10 000 rows x 20 columns
  const cols = Array.from({ length: 20 }, (_, i) => `col_${i}`);
  const rows = Array.from({ length: 10_000 }, (_, n) => ({
    columns: cols,
    values: Object.fromEntries(cols.map((c, i) => [c, `value-${n}-${i}`])),
  }));
  r('16.rowsToJson 10000x20', () => rowsToJson(rows), 3);
  return out;
}

(window as unknown as { __pure: typeof runAll }).__pure = runAll;
