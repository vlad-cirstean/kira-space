// xterm link provider for file paths: each hovered row's candidates are parsed by `filePaths.ts`,
// then checked against the live cwd by the backend; only openable files become links.
import type { IBufferLine, ILink, ILinkProvider, Terminal } from '@xterm/xterm';
import { logWarn } from '../bridge/log';
import { type CellRow, findFilePaths, toCellRange } from './filePaths';
import type { FileLinkTarget, TerminalFileLinks } from './terminalRenderer';

export interface FileLinkHandlers {
  open(event: MouseEvent, target: FileLinkTarget): void;
  hover(event: MouseEvent, text: string): void;
  leave(): void;
}

function cellRow(line: IBufferLine): CellRow {
  const chars: number[] = [];
  const widths: number[] = [];
  let cell = line.getCell(0);
  for (let x = 0; x < line.length; x++) {
    cell = line.getCell(x, cell);
    if (!cell) break;
    const width = cell.getWidth();
    widths.push(width);
    chars.push(width === 0 ? 0 : cell.getChars().length || 1);
  }
  return { chars, widths };
}

export function createFileLinkProvider(
  term: Terminal,
  tabId: string,
  fileLinks: TerminalFileLinks,
  handlers: FileLinkHandlers,
): ILinkProvider {
  return {
    provideLinks(bufferLineNumber, callback) {
      const buf = term.buffer.active;
      const y = bufferLineNumber - 1;
      if (!buf.getLine(y)) return callback(undefined);
      let first = y;
      while (first > 0 && buf.getLine(first)?.isWrapped) first--;
      let last = y;
      while (buf.getLine(last + 1)?.isWrapped) last++;

      const rows: CellRow[] = [];
      let text = '';
      for (let r = first; r <= last; r++) {
        const line = buf.getLine(r);
        if (!line) break;
        rows.push(cellRow(line));
        // Only the final row is trimmed: earlier rows must keep their cells for the index map.
        text += line.translateToString(r === last);
      }

      const candidates = findFilePaths(text).flatMap((m) => {
        const range = toCellRange(rows, m.start, m.end);
        const touches = range && range.startRow <= y - first && y - first <= range.endRow;
        return range && touches ? [{ m, range }] : [];
      });
      if (candidates.length === 0) return callback(undefined);

      fileLinks
        .resolve(
          tabId,
          candidates.map((c) => c.m.path),
        )
        .then(
          (openable) => {
            const links: ILink[] = candidates.flatMap(({ m, range }, i) => {
              if (!openable[i]) return [];
              const matched = text.slice(m.start, m.end);
              const target: FileLinkTarget = { path: m.path, line: m.line, col: m.col };
              return [
                {
                  range: {
                    start: { x: range.startCol + 1, y: first + range.startRow + 1 },
                    end: { x: range.endCol + 1, y: first + range.endRow + 1 },
                  },
                  text: matched,
                  decorations: { underline: true, pointerCursor: true },
                  activate: (event) => handlers.open(event, target),
                  hover: (event) => handlers.hover(event, matched),
                  leave: () => handlers.leave(),
                },
              ];
            });
            callback(links.length > 0 ? links : undefined);
          },
          (err: unknown) => {
            logWarn('terminalFileLinks', 'resolve failed', err);
            callback(undefined);
          },
        );
    },
  };
}
