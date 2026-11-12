import { t } from '@theme/i18n';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import { copyImageOrReportErrorLazy } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import { errorText } from '@workbench/bridge/codedError';
import { type MaybeRefOrGetter, nextTick, toValue } from 'vue';
import { control } from '../bridge/control';
import {
  blobToBase64,
  canvasToJpeg,
  canvasToPng,
  captureElement,
  shotFileName,
} from './screenshot';
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

/** The active tab to the clipboard (PNG), every tab as separate clipboard images, or every tab as
 *  its own JPEG in a fresh folder. */
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

  /** Activates each tab in turn, one in memory at a time, and restores the original tab. Returns the
   *  tab count done. A failure after the first tab is wrapped by `partial` when given. */
  async function walkTabs(
    each: (tab: ProofTab, i: number, total: number) => Promise<void>,
    partial?: (done: number, total: number, e: unknown) => Error,
  ): Promise<number> {
    const tabs = toValue(source.tabs);
    if (tabs.length === 0) throw new Error(t('studio.proof.shot.none.error'));
    const original = store.activeId;
    let done = 0;
    try {
      for (const [i, tab] of tabs.entries()) {
        store.activeId = tab.id;
        await nextTick();
        await each(tab, i, tabs.length);
        done += 1;
      }
    } catch (e) {
      if (done === 0 || !partial) throw e;
      throw partial(done, tabs.length, e);
    } finally {
      store.activeId = original;
    }
    return done;
  }

  /** A failure keeps the files already written. */
  async function saveAll(): Promise<void> {
    const total = toValue(source.tabs).length;
    if (total === 0) throw new Error(t('studio.proof.shot.none.error'));
    const folder = await control.proofOpenShotFolder(toValue(source.sessionName));
    let saved = 0;
    try {
      saved = await walkTabs(
        async (tab, i, n) => {
          const jpeg = await canvasToJpeg(await shoot());
          const name = shotFileName(i + 1, n, tab.state.meta.description);
          await control.proofSaveJpegTo(folder.id, name, await blobToBase64(jpeg));
        },
        (done, n, e) =>
          new Error(
            t('studio.proof.shot.savedPartial.error', {
              count: done,
              total: n,
              error: errorText(e),
            }),
            { cause: e },
          ),
      );
    } finally {
      try {
        await control.proofCloseShotFolder(folder.id);
      } catch {
        // Best effort: Go evicts the oldest folder ids past a small cap.
      }
    }
    store.setShotResult({ kind: 'saved', count: saved, path: folder.path, folderId: folder.id });
    announceApp(t('studio.proof.shot.savedAll.status', { count: saved, path: folder.path }));
  }

  /** Every tab as a PNG into one backend batch, then one clipboard write. Nothing reaches the
   *  clipboard until the commit, so a failure leaves it as it was. */
  async function copyAll(): Promise<void> {
    if (toValue(source.tabs).length === 0) throw new Error(t('studio.proof.shot.none.error'));
    const clip = await control.proofOpenShotClip();
    let count = 0;
    try {
      await walkTabs(async (tab, i, n) => {
        const png = await canvasToPng(await shoot());
        const name = shotFileName(i + 1, n, tab.state.meta.description, 'png');
        await control.proofAddShotClip(clip.id, name, await blobToBase64(png));
      });
      count = (await control.proofCommitShotClip(clip.id)).count;
    } finally {
      try {
        await control.proofCloseShotClip(clip.id);
      } catch {
        // Best effort: the batch dir lives in the OS temp folder.
      }
    }
    store.setShotResult({ kind: 'copied', count });
    announceApp(t('studio.proof.shot.copiedAll.status', { count }));
  }

  type Mode = 'copy' | 'copyAll' | 'saveAll';
  const action = useAsyncAction(async (mode: Mode) => {
    if (mode === 'copy') await copy();
    else if (mode === 'copyAll') await copyAll();
    else await saveAll();
  });

  async function run(mode: Mode): Promise<void> {
    if (action.pending.value) return;
    if (await action.run(mode)) return;
    const message = t('studio.proof.shot.failed.error', { error: action.error.value ?? '' });
    if (mode !== 'copy') store.setShotResult({ kind: 'error', message });
    announceApp(message);
  }

  return {
    busy: action.pending,
    copy: () => run('copy'),
    copyAll: () => run('copyAll'),
    saveAll: () => run('saveAll'),
  };
}
