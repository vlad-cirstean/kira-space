<script setup lang="ts">
import type { KeptUse } from '@shared/domain/kept';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { FieldDescription } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import RelativeTime from '@workbench/components/RelativeTime.vue';
import KeptNameMark from '../kept/KeptNameMark.vue';
import KeptValueView from '../kept/KeptValueView.vue';
import { useKeptExpiry } from '../kept/keptExpiry';
import { keptTypeLabel } from '../kept/keptFormat';
import { useClearKept } from '../kept/keptQueries';

// One variable of the card's panel, compact: name inside the same mark as in the request, the value, then a
// meta line with age, expiry, source and where the request reads it. The countdown is display only; the
// backend decides expiry. The x deletes the stored value.
const props = defineProps<{ use: KeptUse; usedIn: string; clearTestid: string }>();

const expiry = useKeptExpiry(() => props.use.expiresAt);
const clear = useClearKept();
</script>

<template>
  <div class="flex flex-col gap-0.5">
    <div class="flex items-center justify-between gap-2">
      <KeptNameMark :name="use.name" :type="keptTypeLabel(use)" :secret="use.secret" />
      <TooltipIconButton
        v-if="use.id"
        :icon="ICON.close"
        :label="t('studio.kept.clear.tooltip')"
        :aria-label="t('common.action.delete.label')"
        :data-testid="clearTestid"
        :disabled="clear.isPending.value"
        @click="clear.mutate([use.id])"
      />
    </div>
    <KeptValueView :entry="use" />
    <FieldDescription class="flex flex-wrap items-center gap-x-1">
      <span data-testid="kept-age"
        >{{ t('studio.kept.keptAt.label') }} <RelativeTime :at="use.createdAt" /></span
      >
      <span aria-hidden="true">·</span>
      <span data-testid="kept-expiry">{{ expiry }}</span>
      <template v-if="usedIn">
        <span aria-hidden="true">·</span>
        <span
          >{{ t('studio.kept.usedIn.label') }}
          <span data-testid="kept-used-in">{{ usedIn }}</span></span
        >
      </template>
      <span aria-hidden="true">·</span>
      <Badge v-if="use.source.kind === 'user'" variant="info" data-testid="kept-supplied">{{
        t('studio.kept.supplied.status')
      }}</Badge>
      <span v-else class="font-data">
        <TruncatedText :text="use.source.request" />
        <span v-if="use.source.client">{{ use.source.client }}</span>
      </span>
    </FieldDescription>
  </div>
</template>
