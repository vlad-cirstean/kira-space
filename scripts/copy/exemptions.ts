import type { Exemption } from './allowlist';

/** Permanent check-copy exemptions: text the guard flags that is not user-facing wording. */
export const EXEMPTIONS: readonly Exemption[] = [
  {
    rule: 'U67',
    path: 'packages/shared/protocol/frame.ts',
    reason: 'protocol decode diagnostic, same class as backend error text (P266)',
  },
  {
    rule: 'U67',
    path: 'apps/kira-studio/frontend/src/views/console/completion.ts',
    reason: 'Redis command syntax hints (command grammar, not wording)',
  },
  {
    rule: 'U67',
    path: 'apps/kira-studio/frontend/src/views/shared/document/ejson.ts',
    reason: 'EJSON shell-literal syntax (ObjectId(...), ISODate(...)), not wording',
  },
  {
    rule: 'U64',
    path: 'apps/kira-space/frontend/src/ade/v2/AdeActivityIcon.vue',
    reason: '`z` is the waiting-on-monitor glyph, not text',
  },
  {
    rule: 'U64',
    path: 'packages/workbench/src/automations/run/RunPreview.vue',
    reason: '`--tools` and `--allowedTools` are claude CLI flag names, shown as the command line',
  },
];
