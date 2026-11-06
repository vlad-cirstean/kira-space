import { expect, type Locator } from '@playwright/test';

// Monaco virtualises lines — `.view-lines` only ever contains the currently
// rendered subset of a large document, unlike CodeMirror's `.cm-content` (whose `innerText` was
// the whole document) before it. `MonacoHost.vue` exposes its model's full text through a
// `__KIRA_DEBUG_HOOKS__`-gated `data-kira-editor-text` attribute on its own root element — the
// same convention `vite.config.ts`'s `__KIRA_DEBUG_HOOKS__` define and `main.ts`'s `window.__kira*`
// hooks already use, rather than a `page.evaluate` over a `window.monaco` global that isn't
// otherwise exposed. This is the one place every UI spec reads a Monaco host's full document from.

/** `container` is any Locator either wrapping exactly one `MonacoHost` mount (a pane, a dialog
 *  body, a row whose own `data-testid` sits on an ancestor — `.monaco-host` is then a descendant)
 *  or the mount's own root directly (a site that passes its `data-testid` straight onto
 *  `<MonacoHost>`, which now lands on that root itself via `$attrs` fallthrough — `container` IS
 *  `.monaco-host` there, not its parent). `descendant-or-self` covers both without the caller
 *  needing to know which shape its own testid produced. Finds that host's own root and reads its
 *  full text; empty string if the attribute is absent (host still pending its first paint, or not
 *  a MonacoHost at all). */
export async function editorText(container: Locator): Promise<string> {
  const host = container.locator(
    'xpath=./descendant-or-self::*[contains(concat(" ", normalize-space(@class), " "), " monaco-host ")]',
  );
  // `getAttribute` alone does not retry on the *value* — the host element itself renders
  // synchronously (`.monaco-host`'s own div is the host's single root, present even while still
  // `pending`), but `data-kira-editor-text` isn't set until `loadMonaco()` resolves
  // and the model exists. `toHaveAttribute` with a catch-all pattern auto-retries exactly that —
  // present at all (any value, including a legitimately empty document) — before the plain read.
  await expect(host).toHaveAttribute('data-kira-editor-text', /^[\s\S]*$/);
  return (await host.getAttribute('data-kira-editor-text')) ?? '';
}

/** `ResponseDiffDialog.vue`'s own diff editor — its two sides are plain Monaco sub-editors, not a
 *  `MonacoHost` mount, so they carry no `data-kira-editor-text` of their own; the dialog sets a
 *  pair of `data-kira-diff-{original,modified}-text` attributes directly on the diff host once its
 *  (static, read-only, never re-set) models are created. `container` is the diff host element
 *  itself (`[data-testid="http-diff-merge"]`). */
export async function diffEditorText(
  container: Locator,
): Promise<{ original: string; modified: string }> {
  await expect(container).toHaveAttribute('data-kira-diff-original-text', /^[\s\S]*$/);
  const [original, modified] = await Promise.all([
    container.getAttribute('data-kira-diff-original-text'),
    container.getAttribute('data-kira-diff-modified-text'),
  ]);
  return { original: original ?? '', modified: modified ?? '' };
}

/** `locator.boundingBox()`, retried: a `{{variable}}` decoration span (`.kira-ed-var` and
 *  siblings) inside a live MonacoHost can briefly vanish and come back as a *new* DOM node —
 *  `scheduleLint`'s own debounced `setModelMarkers` pass re-renders `.view-lines`, independent of
 *  and shortly after the decorations that painted the span in the first place. A `toHaveCount(1)`
 *  check right before a plain `boundingBox()` call can pass, then still catch the node mid-churn:
 *  present, but not (yet) laid out, so `boundingBox()` returns `null`. Polling re-resolves the
 *  locator each attempt, so it naturally finds the node's replacement once one lands. */
export async function stableBoundingBox(
  locator: Locator,
): Promise<{ x: number; y: number; width: number; height: number }> {
  let box: Awaited<ReturnType<Locator['boundingBox']>> = null;
  await expect
    .poll(async () => {
      box = await locator.boundingBox();
      return box !== null;
    })
    .toBe(true);
  if (!box) throw new Error('stableBoundingBox: locator never settled on a box');
  return box;
}
