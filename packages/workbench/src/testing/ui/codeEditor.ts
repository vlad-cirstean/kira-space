import { expect, type Locator, type Page } from '@playwright/test';

// Shared steps for specs that type into a CodeTextEditor. Monaco virtualises its lines, so the
// full text is read from the debug attribute on the editor root.

/** `host` is the config file's `data-testid` locator, which is the editor root itself. */
function editorRoot(host: Locator): Locator {
  return host
    .locator('xpath=self::*[contains(@class,"monaco-host")] | .//*[contains(@class,"monaco-host")]')
    .first();
}

export async function codeEditorText(host: Locator): Promise<string> {
  const root = editorRoot(host);
  await expect(root).toHaveAttribute('data-kira-editor-text', /^[\s\S]*$/);
  return (await root.getAttribute('data-kira-editor-text')) ?? '';
}

/** Replaces the whole text of the editor. */
export async function replaceCodeEditorText(
  page: Page,
  host: Locator,
  text: string,
): Promise<void> {
  await editorRoot(host).locator('.view-lines').click();
  // The UI test pages report macOS, so Select All is Cmd+A.
  await page.keyboard.press('Meta+a');
  await page.keyboard.insertText(text);
  await expect.poll(() => codeEditorText(host)).toBe(text);
}

/** The editor root has the language, and one line paints at least two distinct token colours
 *  (a key and a value). */
export async function expectHighlighted(host: Locator, language: string): Promise<void> {
  const root = editorRoot(host);
  await expect(root).toHaveAttribute('data-language', language);
  await expect
    .poll(() =>
      root.locator('.view-line').evaluateAll((lines) => {
        for (const line of lines) {
          const colours = new Set(
            [...line.querySelectorAll('span > span')].map((s) => getComputedStyle(s).color),
          );
          if (colours.size > 1) return true;
        }
        return false;
      }),
    )
    .toBe(true);
}
