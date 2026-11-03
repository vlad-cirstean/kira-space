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

/** True for a Jira issue link: `/browse/<KEY>`, `selectedIssue=<KEY>`, or any link on the host of the
 *  task's known Jira link. Other URLs holding a key-shaped token (a branch, a page title) are not. */
export function isJiraIssueUrl(raw: string, knownJiraUrl = ''): boolean {
  let url: URL;
  try {
    url = new URL(raw.trim());
  } catch {
    return false;
  }
  if (new RegExp(`/browse/${JIRA_KEY_RE.source}(?:[/?#]|$)`).test(url.pathname)) return true;
  if (new RegExp(`^${JIRA_KEY_RE.source}$`).test(url.searchParams.get('selectedIssue') ?? '')) {
    return true;
  }
  try {
    return knownJiraUrl !== '' && new URL(knownJiraUrl).host === url.host;
  } catch {
    return false;
  }
}
