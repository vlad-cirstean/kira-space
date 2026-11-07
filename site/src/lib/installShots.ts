import type { ImageMetadata } from 'astro';

type Glob = Record<string, { default: ImageMetadata }>;

// User-supplied dialog screenshot, committed.
const files: Glob = import.meta.glob('../assets/install/*.webp', { eager: true });

export interface InstallShot {
  readonly src: string;
  readonly width: number;
  readonly height: number;
}

/** Throws on a missing file: the image is committed, so the build never ships without it. */
export function installShot(file: string): InstallShot {
  const hit = Object.entries(files).find(([path]) => path.endsWith(`/install/${file}`));
  if (!hit)
    throw new Error(
      `Missing install image site/src/assets/install/${file}. Add the user-supplied dialog screenshot (WebP, cropped to the dialog).`,
    );
  const { src, width, height } = hit[1].default;
  return { src, width, height };
}
