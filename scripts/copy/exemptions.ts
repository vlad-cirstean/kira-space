import type { Exemption } from './allowlist';
import type { OwnershipPair } from './ownership';

/** Permanent check-copy exemptions: text the guard flags that is not user-facing wording. */
export const EXEMPTIONS: readonly Exemption[] = [
  {
    rule: 'U67',
    path: 'packages/shared/protocol/frame.ts',
    reason: 'protocol decode diagnostic, same class as backend error text (P266)',
  },
  {
    rule: 'U67',
    path: 'apps/kira-space/frontend/src/studio/views/console/completion.ts',
    reason: 'Redis command syntax hints (command grammar, not wording)',
  },
  {
    rule: 'U67',
    path: 'apps/kira-space/frontend/src/studio/views/shared/document/ejson.ts',
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
  {
    rule: 'U67',
    path: 'packages/theme/src/components/ui/sonner/Sonner.vue',
    reason:
      '`title` and `description` are vue-sonner class slots holding Tailwind classes, not text',
  },
  {
    rule: 'U208',
    path: 'apps/kira-space/frontend/src/ade/v2/board/setupGate.ts',
    reason: 'mirrors Go run notes (internal/ade/runs.go), matched with includes, never shown',
  },
  {
    rule: 'U208',
    path: 'apps/kira-space/frontend/src/ade/v2/dialog/compose.ts',
    reason: 'agent prompt text sent to the agent, English by design',
  },
];

/** U182 pairs: a module that hosts another module's UI reads that module's keys. */
export const OWNERSHIP_PAIRS: readonly OwnershipPair[] = [
  {
    module: 'mobile',
    prefix: 'space.',
    reason: 'the phone app renders the Space screens with Space wording',
  },
  {
    module: 'space',
    prefix: 'git.',
    reason: 'ADE hosts git-ui review comments and git operations',
  },
  {
    module: 'space',
    prefix: 'studio.proof.',
    reason: 'ADE renders Studio proofs',
  },
  {
    module: 'shell',
    prefix: 'studio.settings.',
    reason: 'the settings dialog nav and app panes host Studio fields',
  },
  {
    module: 'shell',
    prefix: 'studio.mode.',
    reason: 'the mode switcher names Studio modes',
  },
  {
    module: 'shell',
    prefix: 'studio.operations.',
    reason: 'the op log hosts Studio operation rows',
  },
  {
    module: 'shell',
    prefix: 'studio.tab.',
    reason: 'the tab host wires the Studio tab kinds incognito marker',
  },
  {
    module: 'shell',
    prefix: 'llmPrompts.nav.',
    reason: 'the settings nav names the prompts pane',
  },
];
