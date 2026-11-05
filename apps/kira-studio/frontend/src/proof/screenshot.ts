import { domToCanvas } from 'modern-screenshot';
import { TOOLTIP_CLASS } from './chartOption';

/** Every real CSS property, none of the custom ones. The page defines thousands of `--kira-*` and
 *  `--tw-*` properties that every element inherits; copying them into each clone made a capture
 *  take minutes, and the resolved values are already on the real properties. */
function paintedProperties(): string[] {
  const style = getComputedStyle(document.documentElement);
  return Array.from({ length: style.length }, (_, i) => style.item(i)).filter(
    (name) => !name.startsWith('--'),
  );
}

/** Renders the element as it is on screen, scrolled lists included: `restoreScrollPosition` shifts
 *  each scrolled container's children by its offset in the clone. */
export function captureElement(el: HTMLElement): Promise<HTMLCanvasElement> {
  return domToCanvas(el, {
    scale: window.devicePixelRatio || 1,
    backgroundColor: getComputedStyle(el).backgroundColor,
    includeStyleProperties: paintedProperties(),
    features: { restoreScrollPosition: true },
    filter: (node) => !(node instanceof Element && node.classList.contains(TOOLTIP_CLASS)),
  });
}

export function canvasToPng(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error('canvasToPng: encode failed'))),
      'image/png',
    );
  });
}

export async function blobToBase64(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = '';
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary);
}

/** A file-name-safe slug of a session or step name. */
export function fileSlug(name: string): string {
  return (
    name
      .normalize('NFKD')
      .replace(/[^\w.-]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 80) || 'proof'
  );
}

/** `NN-step-name.png` for the tab at 1-based `pos`, zero-padded to the tab count's width. */
export function shotFileName(pos: number, total: number, description: string): string {
  const width = Math.max(2, String(total).length);
  return `${String(pos).padStart(width, '0')}-${fileSlug(description)}.png`;
}
