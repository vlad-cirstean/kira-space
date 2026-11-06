import { getImage } from 'astro:assets';
import type { ImageMetadata } from 'astro';
import { SHOTS, type ShotDef } from '../data/shots';

type Glob = Record<string, { default: ImageMetadata }>;

const framed: Glob = import.meta.glob('../../../screenshots/framed/*.webp', { eager: true });
const detail: Glob = import.meta.glob('../../../screenshots/*-detail.webp', { eager: true });

function find(glob: Glob, file: string): ImageMetadata {
  const hit = Object.entries(glob).find(([path]) => path.endsWith(`/${file}`));
  if (!hit) throw new Error(`Missing screenshot ${file}. Run: bun run screenshots`);
  return hit[1].default;
}

export interface ResolvedShot extends ShotDef {
  readonly framed: string;
  readonly framedWidth: number;
  readonly framedHeight: number;
  readonly detail: string;
  readonly detailWidth: number;
  readonly detailHeight: number;
  /** Build-time PNG of the framed shot for og:image; scrapers handle WebP unevenly. Not committed. */
  readonly social: string;
}

/** Resolves one listed shot; throws at build time when a file is missing. */
export async function resolveShot(id: string): Promise<ResolvedShot> {
  const def = SHOTS.find((s) => s.id === id);
  if (!def) throw new Error(`Unknown shot ${id}`);
  const framedMeta = find(framed, `${id}.webp`);
  const detailMeta = find(detail, `${id}-detail.webp`);
  const social = await getImage({ src: framedMeta, format: 'png' });
  return {
    ...def,
    framed: framedMeta.src,
    framedWidth: framedMeta.width,
    framedHeight: framedMeta.height,
    detail: detailMeta.src,
    detailWidth: detailMeta.width,
    detailHeight: detailMeta.height,
    social: social.src,
  };
}

/** Fails the build when any listed shot has no file. */
export async function resolveAllShots(): Promise<ResolvedShot[]> {
  return Promise.all(SHOTS.map((s) => resolveShot(s.id)));
}
