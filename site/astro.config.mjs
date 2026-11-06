import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'astro/config';

const REPO_URL = 'https://github.com/vlad-cirstean/kira-studio';

export default defineConfig({
  site: 'https://vlad-cirstean.github.io',
  base: '/kira-studio',
  trailingSlash: 'ignore',
  redirects: { '/docs/shared/automations': '/kira-studio/docs/shared/runs/' },
  output: 'static',
  vite: { plugins: [tailwindcss()] },
  integrations: [
    starlight({
      title: 'Kira Studio',
      logo: { src: '../apps/kira-space/build/appicon.icon/Assets/kira_icon_vector.svg', alt: '' },
      favicon: '/favicon.svg',
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
        { label: 'Kira Studio', items: [{ autogenerate: { directory: 'docs/studio' } }] },
        { label: 'Kira Space', items: [{ autogenerate: { directory: 'docs/space' } }] },
        { label: 'Both apps', items: [{ autogenerate: { directory: 'docs/shared' } }] },
        { label: 'Reference', items: [{ autogenerate: { directory: 'docs/reference' } }] },
      ],
    }),
  ],
});
