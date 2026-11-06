import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { copyImageOrReportErrorLazy } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { errorText } from '@workbench/bridge/codedError';
import { type MaybeRefOrGetter, nextTick, toValue } from 'vue';
import { control } from '../bridge/control';
import { blobToBase64, canvasToPng, captureElement, fileSlug, shotFileName } from './screenshot';
import { useProofWindowStore } from './state';
import type { ProofTab } from './tabHost';

const READY_TIMEOUT_MS = 15_000;

const frames = (n: number): Promise<void> =>
  new Promise((resolve) => {
    const step = (left: number): void => {
      if (left === 0) resolve();
      else requestAnimationFrame(() => step(left - 1));
    };
    step(n);
  });

/** Resolves once every step view under `root` has restored its saved scroll and highlight and
 *  painted, so the capture shows exactly what the screen settles on. */
async function whenSettled(root: HTMLElement): Promise<void> {
  const deadline = performance.now() + READY_TIMEOUT_MS;
  for (;;) {
    const views = root.querySelectorAll('[data-step-key]');
    if (views.length > 0 && [...views].every((v) => v.getAttribute('data-ready') === 'true')) break;
    if (performance.now() > deadline) throw new Error(t('studio.proof.shot.notSettled.error'));
    await frames(1);
  }
  await frames(2);
}

interface Source {
  /** The pane to photograph: tab strip and body. */
  target: MaybeRefOrGetter<HTMLElement | null>;
  tabs: MaybeRefOrGetter<ProofTab[]>;
  sessionName: MaybeRefOrGetter<string>;
}

/** The active tab to the clipboard or a PNG, or every tab as its own PNG in a fresh folder. */
export function useProofScreenshot(source: Source) {
  const store = useProofWindowStore();

  async function shoot(): Promise<HTMLCanvasElement> {
    const el = toValue(source.target);
    if (!el) throw new Error('shoot: no target');
    await whenSettled(el);
    return captureElement(el);
  }

  async function copy(): Promise<void> {
    let failure: string | null = null;
    await copyImageOrReportErrorLazy(shoot().then(canvasToPng), (message) => {
      failure = message;
    });
    if (failure) throw new Error(failure);
    announceApp(t('studio.proof.shot.copied.announce'));
  }

  async function saveActive(): Promise<void> {
    const png = await canvasToPng(await shoot());
    const only = toValue(source.tabs).find((tab) => tab.active);
    const base = fileSlug(toValue(source.sessionName));
    const name = only ? `${base}-${String(only.state.meta.ordinal).padStart(2, '0')}` : base;
    const result = await control.proofSavePng(`${name}.png`, await blobToBase64(png));
    if (result.saved && result.path) {
      announceApp(t('studio.proof.shot.saved.announce', { path: result.path }));
    }
  }

  /** One tab in memory at a time; a failure keeps the files already written. */
  async function saveAll(): Promise<void> {
    const tabs = toValue(source.tabs);
    const folder = await control.proofChooseShotFolder(toValue(source.sessionName));
    if (folder.canceled) return;
    const original = store.activeId;
    let saved = 0;
    try {
      for (const [i, tab] of tabs.entries()) {
        store.activeId = tab.id;
        await nextTick();
        const png = await canvasToPng(await shoot());
        const name = shotFileName(i + 1, tabs.length, tab.state.meta.description);
        await control.proofSavePngTo(folder.id, name, await blobToBase64(png));
        saved += 1;
      }
    } catch (e) {
      if (saved === 0) throw e;
      throw new Error(
        t('studio.proof.shot.savedPartial.error', {
          count: saved,
          total: tabs.length,
          error: errorText(e),
        }),
        { cause: e },
      );
    } finally {
      store.activeId = original;
      try {
        await control.proofForgetShotFolder(folder.id);
      } catch {
        // Best effort: Go evicts the oldest folder ids past a small cap.
      }
    }
    announceApp(t('studio.proof.shot.savedAll.announce', { count: saved, path: folder.path }));
  }

  const action = useAsyncAction(async (mode: 'copy' | 'save' | 'saveAll') => {
    if (mode === 'copy') await copy();
    else if (mode === 'save') await saveActive();
    else await saveAll();
  });

  async function run(mode: 'copy' | 'save' | 'saveAll'): Promise<void> {
    if (action.pending.value) return;
    if (!(await action.run(mode))) {
      announceApp(t('studio.proof.shot.failed.error', { error: action.error.value ?? '' }));
    }
  }

  return {
    busy: action.pending,
    copy: () => run('copy'),
    save: () => run('save'),
    saveAll: () => run('saveAll'),
  };
}
