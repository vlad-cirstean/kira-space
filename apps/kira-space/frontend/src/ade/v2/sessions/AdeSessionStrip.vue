<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { tabChipVariants } from '@theme/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { revealElement } from '@workbench/behaviour/focus';
import { onMounted, ref, watch } from 'vue';
import { useMobileTerminalsStore } from '../../../state/mobileTerminals';
import AdeActivityIcon from '../AdeActivityIcon.vue';
import AdeOpStatus from '../plan/AdeOpStatus.vue';
import AdeSessionBadge from './AdeSessionBadge.vue';
import AdeSessionTabTip from './AdeSessionTabTip.vue';
import type { SessionView } from './sessionView';
import { useSessionMenu } from './useSessionMenu';

// Tab strip of the running sessions in scope (mockup `termTabs`): activity glyph, TUI / headless
// badge, name; hover for state and age, right-click for Restart, Close and Copy session ID.
const props = defineProps<{ views: SessionView[]; selectedId: string | null }>();
const emit = defineEmits<{ select: [id: string] }>();
const phones = useMobileTerminalsStore();
const menu = useSessionMenu();
const strip = ref<HTMLElement | null>(null);

// The strip scrolls without a bar, so a pick from the dropdown can land on a hidden tab.
function revealSelected(): void {
  const tab = strip.value?.querySelector('[data-selected]');
  if (tab) revealElement(tab);
}
onMounted(revealSelected);
watch(() => props.selectedId, revealSelected, { flush: 'post' });
</script>

<template>
  <div
    ref="strip"
    class="flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto scrollbar-none px-1.5 py-1"
    data-testid="ade-session-strip"
  >
    <Tooltip v-for="v in views" :key="v.session.id">
      <TooltipTrigger as-child>
        <Button
          type="button"
          variant="toolbar"
          size="kira-lg"
          :class="[
            tabChipVariants({ active: v.session.id === selectedId }),
            'font-data overflow-hidden',
          ]"
          :data-testid="`ade-session-tab-${v.session.id}`"
          :data-selected="v.session.id === selectedId || undefined"
          :data-mode="v.session.mode"
          @click="emit('select', v.session.id)"
          @contextmenu="(ev: MouseEvent) => menu.open(ev, v)"
        >
          <AdeActivityIcon :kind="v.kind" bare />
          <AdeSessionBadge :view="v" />
          <span class="min-w-0 truncate">{{ v.tabName }}</span>
          <AdeOpStatus :session-id="v.session.id" />
          <CodiconIcon
            v-if="phones.holdOf(v.session.terminalId)"
            name="device-mobile"
            :size="12"
            class="shrink-0 text-muted-foreground"
            data-testid="ade-session-phone"
          />
        </Button>
      </TooltipTrigger>
      <TooltipContent><AdeSessionTabTip :view="v" /></TooltipContent>
    </Tooltip>
  </div>
</template>
