<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import InlineState from '@theme/components/InlineState.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { TONE_TEXT } from '@theme/lib/statusTone';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import PrGlyph from '../ade/v2/pr/PrGlyph.vue';
import { CHECK_GLYPH, REVIEW_GLYPH } from '../ade/v2/pr/prGlyph';
import { control } from '../bridge/control';
import type { ChangeRequestDetail, Check } from './wire';

defineProps<{ detail: ChangeRequestDetail }>();

function openCheck(c: Check): void {
  if (c.url) fireAndForget(control.openExternal(c.url), 'hostView.check');
}
</script>

<template>
  <aside
    class="flex w-72 shrink-0 flex-col gap-3 overflow-y-auto p-3"
    data-testid="host-view-sidebar"
  >
    <section class="flex flex-col gap-1" data-testid="host-view-status">
      <PrGlyph variant="inline" :status="detail" line-prefix="host-view" />
    </section>

    <section v-if="detail.stats" class="flex flex-col gap-1" data-testid="host-view-stats">
      <p class="m-0 text-kira-sm">{{
        t('space.hostView.stats.summary.label', {
          commits: detail.stats.commits,
          files: detail.stats.files,
          additions: detail.stats.additions,
          deletions: detail.stats.deletions,
        })
      }}</p>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-reviewers">
      <SectionHeading :label="t('space.hostView.reviewers.heading.title')" />
      <InlineState kind="empty" v-if="detail.reviewers.length === 0">{{
        t('space.hostView.reviewers.empty.empty')
      }}</InlineState>
      <div
        v-for="r in detail.reviewers"
        :key="r.person.login"
        class="flex items-center gap-2 text-kira-sm"
        :data-state="r.state"
        data-testid="host-view-reviewer"
      >
        <CodiconIcon
          :name="REVIEW_GLYPH[r.state].icon"
          :size="13"
          :class="TONE_TEXT[REVIEW_GLYPH[r.state].tone]"
        />
        <span class="min-w-0 flex-1 truncate">{{ r.person.name || r.person.login }}</span>
        <Badge v-if="r.required">{{ t('common.required.label') }}</Badge>
        <span class="shrink-0 text-muted-foreground">{{ t(REVIEW_GLYPH[r.state].label) }}</span>
      </div>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-checks">
      <SectionHeading :label="t('space.hostView.checks.heading.title')" />
      <InlineState kind="empty" v-if="detail.checks.length === 0">{{
        t('space.hostView.checks.empty.empty')
      }}</InlineState>
      <div
        v-for="c in detail.checks"
        :key="c.name"
        class="flex items-center gap-2 text-kira-sm"
        :data-state="c.state"
        data-testid="host-view-check"
      >
        <CodiconIcon
          :name="CHECK_GLYPH[c.state].icon"
          :size="13"
          :class="TONE_TEXT[CHECK_GLYPH[c.state].tone]"
        />
        <button
          v-if="c.url"
          type="button"
          class="min-w-0 flex-1 truncate border-0 bg-transparent p-0 text-left text-fg hover:underline"
          @click="openCheck(c)"
          >{{
            c.name
          }}</button
        >
        <span v-else class="min-w-0 flex-1 truncate">{{ c.name }}</span>
        <Badge v-if="c.required">{{ t('common.required.label') }}</Badge>
        <span class="shrink-0 text-muted-foreground">{{ t(CHECK_GLYPH[c.state].label) }}</span>
      </div>
      <FieldDescription v-if="detail.checksTruncated" data-testid="host-view-checks-truncated">{{
        t('space.hostView.checks.truncated.hint')
      }}</FieldDescription>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-labels">
      <SectionHeading :label="t('space.hostView.labels.heading.title')" />
      <InlineState kind="empty" v-if="detail.labels.length === 0">{{
        t('space.hostView.labels.empty.empty')
      }}</InlineState>
      <div class="flex flex-wrap gap-1">
        <Badge v-for="l in detail.labels" :key="l.name" data-testid="host-view-label">
          <StatusDot v-if="l.color" :color="`#${l.color}`" />
          {{ l.name }}
        </Badge>
      </div>
    </section>

    <section class="flex flex-col gap-1.5" data-testid="host-view-assignees">
      <SectionHeading :label="t('space.hostView.assignees.heading.title')" />
      <InlineState kind="empty" v-if="detail.assignees.length === 0">{{
        t('space.hostView.assignees.empty.empty')
      }}</InlineState>
      <span
        v-for="a in detail.assignees"
        :key="a.login"
        class="text-kira-sm"
        data-testid="host-view-assignee"
        >{{
          a.name || a.login
        }}</span
      >
    </section>
  </aside>
</template>
