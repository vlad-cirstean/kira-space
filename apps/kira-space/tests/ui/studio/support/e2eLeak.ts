import { expect, type Page } from '@playwright/test';

/** Everything the page shows or keeps, as named strings the value must not appear in. */
async function surfaces(page: Page, logs: readonly string[]): Promise<Record<string, string>> {
  const dom = await page.evaluate(() => ({
    body: document.body.innerText,
    inputs: [...document.querySelectorAll('input, textarea')]
      .map((el) => (el as HTMLInputElement).value)
      .join('\n'),
    local: JSON.stringify({ ...localStorage }),
    session: JSON.stringify({ ...sessionStorage }),
  }));
  return { ...dom, console: logs.join('\n') };
}

/**
 * Fails when value shows anywhere in the page: body text, every input and textarea value,
 * localStorage, sessionStorage or console lines. Call after every answer that carried a typed value.
 * logs is the array the spec fills from `page.on('console', (m) => logs.push(m.text()))`.
 */
export async function expectNoLeak(
  page: Page,
  value: string,
  logs: readonly string[],
): Promise<void> {
  expect(value, 'leak check needs a value').not.toBe('');
  const forms = [value, JSON.stringify(value).slice(1, -1)];
  for (const [where, text] of Object.entries(await surfaces(page, logs))) {
    for (const form of forms) {
      expect(text.includes(form), `${where} holds the typed value`).toBe(false);
    }
  }
}
