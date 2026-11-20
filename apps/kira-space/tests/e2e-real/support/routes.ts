import type { Page } from '@playwright/test';

const DIALOG_METHODS = ['FilesService.ChooseFolder'];

/**
 * A native dialog has no server-build equivalent. Fails the call loudly instead of letting the page
 * hang on a dialog nobody answers; every other call passes through to the real server.
 */
export async function stubNativeDialogs(page: Page): Promise<void> {
  await page.route('**/wails/runtime', async (route) => {
    const body = JSON.parse(route.request().postData() ?? '{}') as {
      args?: { methodName?: string };
    };
    const name = body.args?.methodName ?? '';
    if (DIALOG_METHODS.some((m) => name.endsWith(m))) {
      await route.fulfill({
        status: 422,
        contentType: 'application/json',
        body: JSON.stringify({
          cause: { code: 'E_NO_DIALOG', message: 'native dialogs are not available in e2e-real' },
        }),
      });
      return;
    }
    await route.fallback();
  });
}
