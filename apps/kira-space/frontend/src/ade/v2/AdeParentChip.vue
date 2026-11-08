<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import AdeTip from './AdeTip.vue';
import type { ParentModel } from './plan/usePlanModel';
import { TONE_TAG_CLASS } from './tones';

// A child's link to its orchestrator. An archived parent shows muted and does not select: it is no
// longer on the board.
defineProps<{ parent: ParentModel }>();
const emit = defineEmits<{ select: [] }>();
</script>

<template>
  <AdeTip
    :text="
      parent.archived
        ? t('space.ade.orchestrator.parent.archived.tooltip', { title: parent.title })
        : t('space.ade.orchestrator.parent.tooltip', { title: parent.title })
    "
  >
    <Badge
      v-if="parent.archived"
      variant="chip"
      class="max-w-40 min-w-0 shrink"
      :class="TONE_TAG_CLASS.grey"
      data-testid="ade-parent-chip"
      data-archived="true"
    >
      <CodiconIcon name="organization" :size="12" />
      <span class="truncate">{{ parent.title }}</span>
    </Badge>
    <Badge
      v-else
      as-child
      variant="chip"
      class="max-w-40 min-w-0 shrink"
      :class="TONE_TAG_CLASS.blue"
    >
      <button
        type="button"
        :aria-label="t('space.ade.orchestrator.parent.open.aria', { title: parent.title })"
        data-testid="ade-parent-chip"
        @click.stop="emit('select')"
      >
        <CodiconIcon name="organization" :size="12" />
        <span class="truncate">{{ parent.title }}</span>
      </button>
    </Badge>
  </AdeTip>
</template>
