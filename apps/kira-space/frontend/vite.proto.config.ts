import { fileURLToPath } from 'node:url';
import { defineConfig, mergeConfig } from 'vite';
import { defineAppViteConfig } from '../../../packages/workbench/src/viteAppConfig.ts';

// P165 dev-only prototype build (docs/HISTORY.md (v2.0 P165)). Same aliases,
// plugins and debug-hook define as the app, different entries and outDir. The shipped build
// (`vite build`, index.html only) never reaches `proto/`, and Go embeds only `dist`.
const app = defineAppViteConfig({
  app: 'kira-space',
  port: 9246,
  configUrl: import.meta.url,
});

const page = (name: string): string =>
  fileURLToPath(new URL(`./proto/grid/${name}.html`, import.meta.url));

export default defineConfig(async (env) => {
  const base = await (typeof app === 'function' ? app(env) : app);
  return mergeConfig(base, {
    // One Vue and one cheetah-grid module instance: cheetah-grid-playwright reads the same
    // `window.cheetahGrid` the wrapper builds its grid from.
    resolve: { dedupe: ['vue', 'cheetah-grid'] },
    preview: { host: '127.0.0.1', port: 9246, strictPort: true },
    build: {
      // Hooks builds land beside the release build so a probe can serve both.
      outDir: process.env.KIRA_PROTO_OUT ?? 'dist-proto',
      rolldownOptions: {
        input: { cheetah: page('cheetah'), slick: page('slick'), empty: page('empty') },
      },
    },
  });
});
