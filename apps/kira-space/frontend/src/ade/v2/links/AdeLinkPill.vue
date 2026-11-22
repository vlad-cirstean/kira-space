<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { TONE_DOT } from '@theme/lib/statusTone';
import { cn } from '@theme/lib/utils';
import { artifactStatusLabel, kindIcon } from '../artifacts/status';
import { THEME_TONE } from '../tones';
import { foundTooltip } from './foundTooltip';
import type { LinkItem } from './linkGroups';

// One link as an 18px pill (a Badge): label and a status dot. Detected and found links draw dashed and unfilled; a found link carries a note icon whose tooltip names
// where it was found (the Badge stays the single root, so the popover anchor and roving item still merge onto it).
// A popover anchor or roving-focus item merges onto the Badge root.
withDefaults(defineProps<{ item: LinkItem; showKind?: boolean; as?: 'button' | 'span' }>(), {
  as: 'button',
});
</script>

<template>
  <Badge
    :as="as"
    :type="as === 'button' ? 'button' : undefined"
    :class="
      cn(
        'max-w-48 min-w-0 px-1.5 text-fg focus-visible:focus-ring data-[state=open]:ring-1 data-[state=open]:ring-select-edge',
        (item.origin === 'detected' || item.origin === 'found') &&
          'border border-dashed border-border bg-transparent text-muted-foreground',
      )
    "
    data-testid="ade-links-pill"
    :data-origin="item.origin"
    :data-kind="item.kind"
    :data-url="item.url"
    :data-key="item.key"
  >
    <TextTooltip v-if="item.origin === 'found'" :text="foundTooltip(item.foundIn ?? [])">
      <span class="flex shrink-0" data-testid="ade-links-pill-found">
        <CodiconIcon name="note" :size="12" />
        <span class="sr-only">{{ t('space.ade.links.found.pill.aria') }}</span>
      </span>
    </TextTooltip>
    <CodiconIcon v-if="showKind" :name="kindIcon(item.kind)" :size="12" class="shrink-0" />
    <span class="flex min-w-0" data-testid="ade-links-pill-label"
      ><TruncatedText :text="item.label" /></span
    >
    <template v-if="item.tone">
      <StatusDot
        :tone="TONE_DOT[THEME_TONE[item.tone]]"
        :data-tone="THEME_TONE[item.tone]"
        data-testid="ade-links-pill-dot"
      />
      <span v-if="item.status" class="sr-only">{{ artifactStatusLabel(item.status) }}</span>
    </template>
  </Badge>
</template>
