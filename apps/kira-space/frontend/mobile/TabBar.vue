<script setup lang="ts">
import { Badge } from '@theme/components/ui/badge';
import { t } from '@theme/i18n';
import { useAgentsModel } from './state/useAgentsModel';

// Need You, Later and Plan. Plain links: each tab is its own route, so back and reload work.
const { model } = useAgentsModel();
const TABS = [
  { to: '/needs', label: 'space.common.needYou.label', id: 'needs' },
  { to: '/later', label: 'common.later.label', id: 'later' },
  { to: '/plan', label: 'space.common.plan.label', id: 'plan' },
] as const;
</script>

<template>
  <nav
    class="flex border-t border-border bg-chrome pb-[env(safe-area-inset-bottom)]"
    :aria-label="t('mobile.tabs.nav.aria')"
  >
    <RouterLink
      v-for="tab in TABS"
      :key="tab.id"
      :to="tab.to"
      class="flex min-h-12 flex-1 items-center justify-center gap-1.5 text-kira-md text-muted-foreground no-underline aria-[current=page]:font-medium aria-[current=page]:text-fg"
      :data-testid="`tab-${tab.id}`"
    >
      {{ t(tab.label) }}
      <Badge
        v-if="tab.id === 'needs' && model && model.needs.badge > 0"
        variant="count"
        data-testid="tab-needs-badge"
        >{{
          model.needs.badge
        }}</Badge
      >
    </RouterLink>
  </nav>
</template>
