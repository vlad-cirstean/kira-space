import type { ImageMetadata } from 'astro';

type Glob = Record<string, { default: ImageMetadata }>;

// User-supplied dialog screenshots, committed. Nothing generates them.
const files: Glob = import.meta.glob('../assets/install/*.webp', { eager: true });

export interface InstallShot {
  readonly src: string;
  readonly width: number;
  readonly height: number;
}

const warned = new Set<string>();

/** Returns the image, or undefined locally. Throws under CI=true so Pages never ships a placeholder. */
export function installShot(file: string): InstallShot | undefined {
  const hit = Object.entries(files).find(([path]) => path.endsWith(`/install/${file}`));
  if (hit) {
    const { src, width, height } = hit[1].default;
    return { src, width, height };
  }
  if (process.env.CI === 'true')
    throw new Error(
      `Missing install image site/src/assets/install/${file}. Add the user-supplied dialog screenshot (WebP, cropped to the dialog).`,
    );
  if (!warned.has(file)) {
    warned.add(file);
    console.warn(`Missing install image ${file}, showing a placeholder.`);
  }
  return undefined;
}
