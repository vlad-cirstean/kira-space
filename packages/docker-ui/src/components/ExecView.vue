<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { tabChipVariants } from '@theme/components/ui/tabs';
import { t } from '@theme/i18n';
import TerminalHostView from '@workbench/terminal/TerminalHostView.vue';
import { computed, ref, watch } from 'vue';
import { useDocker } from '../context';
import { useDockerExecSessionsStore } from '../state/dockerExec';

const props = defineProps<{ containerId: string }>();

const ctx = useDocker();
const store = useDockerExecSessionsStore();
const sessions = computed(() => store.forContainer(props.containerId));
const activeId = ref('');
const shownId = computed(() =>
  sessions.value.some((s) => s.id === activeId.value)
    ? activeId.value
    : (sessions.value[0]?.id ?? ''),
);

function openSession(): void {
  activeId.value = store.open(ctx, props.containerId).id;
}

function closeSession(id: string): void {
  store.close(ctx, id);
}

watch(
  () => props.containerId,
  () => {
    activeId.value = sessions.value[0]?.id ?? '';
  },
  { immediate: true },
);

// TerminalHostView needs a tab-shaped state; exec sessions have no cwd.
const hostTab = (id: string) => ({
  id,
  state: { cwd: '', codeRepoId: '', command: '', launchKind: 'shell' as const },
});
</script>

<template>
  <div class="flex h-full flex-col" data-testid="docker-exec">
    <EmptyState
      v-if="sessions.length === 0"
      fill
      data-testid="docker-exec-empty"
      icon="terminal"
      :title="t('docker.exec.none.empty')"
      :description="t('docker.exec.none.emptyHint')"
    >
      <Button size="kira" variant="toolbar" data-testid="docker-exec-new" @click="openSession">
        <CodiconIcon name="add" :size="12" />
        {{ t('common.newSession.label') }}
      </Button>
    </EmptyState>
    <template v-else>
      <div class="flex shrink-0 items-center gap-0.5 px-1.5 py-1">
        <span
          v-for="s in sessions"
          :key="s.id"
          :class="tabChipVariants({ active: s.id === shownId })"
          data-testid="docker-exec-chip"
        >
          <button type="button" class="cursor-default" @click="activeId = s.id">{{
            s.title
          }}</button>
          <TooltipIconButton
            icon="close"
            :icon-size="12"
            :label="t('docker.exec.close.tooltip', { title: s.title })"
            data-testid="docker-exec-close"
            @click="closeSession(s.id)"
          />
        </span>
        <Button variant="toolbar" size="kira" data-testid="docker-exec-new" @click="openSession">
          <CodiconIcon name="add" :size="12" />
          {{ t('common.newSession.label') }}
        </Button>
      </div>
      <div class="relative min-h-0 flex-1">
        <div
          v-for="s in sessions"
          :key="s.id"
          v-show="s.id === shownId"
          class="absolute inset-0"
          :data-testid="`docker-exec-pane`"
        >
          <TerminalHostView :tab="hostTab(s.id)" :deps="ctx.execHostDeps" />
        </div>
      </div>
    </template>
  </div>
</template>
