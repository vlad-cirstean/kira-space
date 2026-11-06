import type { JSONContent } from '@tiptap/core';
import { MarkdownManager } from '@tiptap/markdown';
import { notesExtensions } from './notesExtensions';

// Agent text is plain text with single newlines, which Markdown folds into one line. This parses it
// with each newline kept as a hard break. A per-editor `marked` option cannot do it: `marked`'s
// options are global, so one editor's setting would leak into every other editor.
const manager = new MarkdownManager({ extensions: notesExtensions() });

function splitLines(node: JSONContent): JSONContent[] {
  const { text } = node;
  if (node.type !== 'text' || !text?.includes('\n') || node.marks?.some((m) => m.type === 'code')) {
    return [node];
  }
  return text.split('\n').flatMap((line, i): JSONContent[] => {
    const part: JSONContent[] = line ? [{ ...node, text: line }] : [];
    return i === 0 ? part : [{ type: 'hardBreak' }, ...part];
  });
}

function keepBreaks(node: JSONContent): JSONContent {
  if (node.type === 'codeBlock' || !node.content) return node;
  return {
    ...node,
    content: node.content.flatMap((c) => (c.type === 'text' ? splitLines(c) : [keepBreaks(c)])),
  };
}

/** Parses Markdown into an editor document in which every single newline is a line break. */
export function parseWithLineBreaks(md: string): JSONContent {
  return keepBreaks(manager.parse(md));
}
