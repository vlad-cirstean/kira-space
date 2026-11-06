export interface TextChunkOptions {
  chunkChars: number;
  /** Cut after the next `\n` when it lies within one more chunk of the nominal cut. */
  lineAligned?: boolean;
}

const isHighSurrogate = (c: number): boolean => c >= 0xd800 && c <= 0xdbff;

/** Splits `text` into consecutive slices; concatenating them yields `text`. */
export function* textChunks(text: string, opts: TextChunkOptions): Generator<string> {
  const { chunkChars, lineAligned = false } = opts;
  let pos = 0;
  while (pos < text.length) {
    let end = Math.min(pos + chunkChars, text.length);
    if (end < text.length) {
      const nl = lineAligned ? text.indexOf('\n', end) : -1;
      if (nl !== -1 && nl - end < chunkChars) {
        end = nl + 1;
      } else if (
        isHighSurrogate(text.charCodeAt(end - 1)) ||
        (text.charCodeAt(end - 1) === 13 && text.charCodeAt(end) === 10)
      ) {
        end++;
      }
    }
    yield text.slice(pos, end);
    pos = end;
  }
}

export interface PumpOptions {
  signal?: AbortSignal;
  /** Awaited between chunks; defaults to one animation frame. */
  pace?: () => Promise<void>;
}

const nextFrame = (): Promise<void> => new Promise((r) => requestAnimationFrame(() => r()));

/**
 * Applies each chunk in turn, yielding between them. Resolves true when all chunks were applied,
 * false when `signal` aborted first (no further chunk is applied after the abort).
 */
export async function pumpChunks(
  chunks: Iterable<string>,
  apply: (chunk: string, index: number) => void,
  opts: PumpOptions = {},
): Promise<boolean> {
  const { signal, pace = nextFrame } = opts;
  let index = 0;
  const it = chunks[Symbol.iterator]();
  let cur = it.next();
  while (!cur.done) {
    if (signal?.aborted) return false;
    apply(cur.value, index++);
    cur = it.next();
    if (cur.done) break;
    await pace();
  }
  return !signal?.aborted;
}
