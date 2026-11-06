import { beautifyJson, beautifyXml } from '../../beautify';
import { copyAllDocuments } from '../../views/console/copyAll';
import { formatConsoleText } from '../../views/console/format';
import type { JobInput, JobKind, JobOutput, PrettyResult } from './protocol';

/** One parse per body: detects the format and, when asked, returns the pretty text from that same parse. */
function formatBody(body: string, wantText: boolean): PrettyResult {
  const json = beautifyJson(body, 'indented');
  if (json.ok) return wantText ? { format: 'json', text: json.text } : { format: 'json' };
  // The `<…>` bracket check mirrors celleditor/detect.ts's own detectXml gate: an XML parse alone
  // accepts plain text with no tags at all, so without it every plain-text body reports as XML.
  const t = body.trim();
  if (t.length === 0 || t[0] !== '<' || t[t.length - 1] !== '>') return { format: null };
  const xml = beautifyXml(body, 'indented');
  if (!xml.ok) return { format: null };
  return wantText ? { format: 'xml', text: xml.text } : { format: 'xml' };
}

type Handler<K extends JobKind> = (input: JobInput<K>) => JobOutput<K> | Promise<JobOutput<K>>;

/** Shared by the worker and the client's inline paths, so both run the same code. */
export const handlers = {
  'body.format': ({ body, wantText }) => formatBody(body, wantText),
  'json.beautify': ({ text, mode }) => beautifyJson(text, mode),
  'ejson.copyAll': ({ format, bodies }) => copyAllDocuments(format, bodies),
  'xml.beautify': ({ text, mode }) => beautifyXml(text, mode),
  'console.format': ({ kind, text }) => formatConsoleText(kind, text),
} satisfies { [K in JobKind]: Handler<K> };

/** Kinds whose handler is synchronous, so `parseInline` can run them in the caller's tick. */
export type SyncKind = {
  [K in JobKind]: ReturnType<(typeof handlers)[K]> extends Promise<unknown> ? never : K;
}[JobKind];
