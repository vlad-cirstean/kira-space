import { copyFileSync, existsSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'astro/config';
import { SHOT_CAPTURE, shotFiles } from './src/data/shotCapture.ts';

/** The one site icon: header, favicon, apple-touch icon, docs logo and share image all derive from it. */
const APP_ICON = fileURLToPath(new URL('../apps/kira-space/build/appicon.png', import.meta.url));
const REPO_URL = 'https://github.com/vlad-cirstean/kira-space';

/** Copies each shot's main image to dist/shots/<id>.webp: an unhashed URL the README links to. */
const publishShots = {
  name: 'publish-shots',
  hooks: {
    'astro:build:done': ({ dir }) => {
      const out = fileURLToPath(new URL('shots/', dir));
      mkdirSync(out, { recursive: true });
      for (const id of Object.keys(SHOT_CAPTURE)) {
        const src = fileURLToPath(new URL(`../screenshots/${shotFiles(id).main}`, import.meta.url));
        if (existsSync(src)) copyFileSync(src, `${out}${id}.webp`);
      }
    },
  },
};

export default defineConfig({
  site: 'https://vlad-cirstean.github.io',
  base: '/kira-space',
  trailingSlash: 'ignore',
  redirects: {
    '/docs/shared/automations': '/kira-space/docs/shared/runs/',
    '/space': '/kira-space/',
  },
  output: 'static',
  prefetch: false,
  vite: { define: { __APP_ICON__: JSON.stringify(APP_ICON) }, plugins: [tailwindcss()] },
  integrations: [
    publishShots,
    starlight({
      title: 'Kira Space',
      logo: { src: '../apps/kira-space/build/appicon.png', alt: '' },
      favicon: '/favicon.png',
      social: [{ icon: 'github', label: 'GitHub', href: REPO_URL }],
      customCss: ['./src/styles/docs.css'],
      disable404Route: true,
      components: {
        ThemeSelect: './src/components/Empty.astro',
        ThemeProvider: './src/components/Empty.astro',
      },
      sidebar: [
        { label: 'Overview', link: '/docs/' },
        { label: 'Start', items: [{ autogenerate: { directory: 'docs/start' } }] },
        { label: 'Git, agents and memory', items: [{ autogenerate: { directory: 'docs/space' } }] },
        {
          label: 'Databases, APIs, Docker and Kubernetes',
          items: [{ autogenerate: { directory: 'docs/studio' } }],
        },
        { label: 'Runs', items: [{ autogenerate: { directory: 'docs/shared' } }] },
        { label: 'Reference', items: [{ autogenerate: { directory: 'docs/reference' } }] },
      ],
    }),
  ],
});
