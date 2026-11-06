// db.<collection>.<method>(<args>) — engine/adapters/mongo/console.ts's own grammar (F4). Brackets
// and quotes are checked first and independently of the statement shape: an unterminated string
// makes the rest of the scan meaningless, so nothing else is reported alongside it.
export const MONGO_STATEMENT_RE = /^\s*db\.([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s*\(/;

// P42 D12: finds the matching ')' for the '(' at `openPos` — trusted to exist and to be well
// nested, since this only ever runs after lintMongoBrackets has already found the statement's
// brackets balanced (the same short-circuit lintMongoConsole itself observes below).
export function findMatchingParen(text: string, openPos: number): number {
  let depth = 0;
  for (let i = openPos; i < text.length; i++) {
    const c = text[i];
    if (c === "'" || c === '"') {
      const quote = c;
      i++;
      while (i < text.length && text[i] !== quote) {
        if (text[i] === '\\') i++;
        i++;
      }
      continue;
    }
    if (c === '(') depth++;
    else if (c === ')') {
      depth--;
      if (depth === 0) return i;
    }
  }
  return text.length;
}

// Splits text[start:end) on commas outside any bracket/quote nesting — one span per top-level
// argument, each still carrying its own leading/trailing whitespace (the caller trims).
export function splitTopLevelArgs(
  text: string,
  start: number,
  end: number,
): Array<{ text: string; from: number }> {
  const args: Array<{ text: string; from: number }> = [];
  let depth = 0;
  let argStart = start;
  let i = start;
  while (i < end) {
    const c = text[i];
    if (c === "'" || c === '"') {
      const quote = c;
      i++;
      while (i < end && text[i] !== quote) {
        if (text[i] === '\\') i++;
        i++;
      }
      i++;
      continue;
    }
    if (c === '(' || c === '[' || c === '{') depth++;
    else if (c === ')' || c === ']' || c === '}') depth--;
    else if (c === ',' && depth === 0) {
      args.push({ text: text.slice(argStart, i), from: argStart });
      argStart = i + 1;
      i++;
      continue;
    }
    i++;
  }
  if (argStart < end) args.push({ text: text.slice(argStart, end), from: argStart });
  return args;
}
