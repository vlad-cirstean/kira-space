const JIRA_KEY_RE = /[A-Z][A-Z0-9]+-\d+/;

/** One input takes a bare key or a pasted link; `{key: '', url: ''}` for empty or unrecognised input. */
export function parseJira(input: string): { key: string; url: string } {
  const trimmed = input.trim();
  const key = trimmed.match(JIRA_KEY_RE)?.[0] ?? '';
  let url = '';
  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') url = trimmed;
  } catch {
    // Not a URL: a bare key leaves `url` empty.
  }
  return { key, url };
}
