<script setup lang="ts">
import AdeTagIcon from '@ade/plan/AdeTagIcon.vue';
import { BRANCH_TAG_GLYPH } from '@ade/plan/glyphs';
import PrGlyph from '@ade/pr/PrGlyph.vue';
import { THEME_TONE } from '@ade/tones';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { TONE_TEXT } from '@theme/lib/statusTone';
import type { PhoneBranch } from '../state/useAgentsModel';

// One branch of an expanded card: tag glyph and label, repo, ahead and behind, uncommitted count,
// and the change request through the shared PrGlyph.
const props = defineProps<{ branch: PhoneBranch }>();
</script>

<template>
  <li
    class="flex flex-col gap-1 text-kira-sm"
    data-testid="phone-branch"
    :data-branch-id="branch.id"
  >
    <span class="flex min-w-0 items-center gap-2">
      <AdeTagIcon
        :glyph="BRANCH_TAG_GLYPH[branch.tag.kind]"
        :kind="branch.tag.kind"
        :label="branch.tagText"
        :tip="branch.tag.tip"
        :tip-parts="branch.tag.tipParts"
        :tone="branch.tag.tone"
      />
      <span
        class="min-w-0 flex-1 truncate font-data text-kira-md text-fg"
        data-testid="phone-branch-name"
        >{{
          branch.name
        }}</span
      >
      <Badge>{{ branch.repo }}</Badge>
    </span>
    <span class="flex flex-wrap items-center gap-x-3 gap-y-1 pl-7 text-muted-foreground">
      <span data-testid="phone-branch-tag" :class="TONE_TEXT[THEME_TONE[branch.tag.tone]]">{{
        branch.tagText
      }}</span>
      <span
        v-if="branch.ahead || branch.behind"
        class="font-data"
        data-testid="phone-branch-facts"
        >{{
          t('mobile.plan.branchFacts.label', { ahead: branch.ahead, behind: branch.behind })
        }}</span
      >
      <span v-if="branch.dirty" data-testid="phone-branch-dirty">{{
        t('space.ade.agentDialog.riskUncommitted.label', { count: branch.dirty })
      }}</span>
      <PrGlyph
        v-if="branch.pr"
        variant="inline"
        :status="branch.pr"
        :base-ref="branch.pr.baseRef"
        line-prefix="phone-branch-pr"
        testid="phone-branch-pr"
      />
    </span>
  </li>
</template>
