<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { useDebounceFn, useEventListener, useResizeObserver } from '@vueuse/core';
import { usePersistedState } from '@workbench/behaviour/persisted';
import { TIMING } from '@workbench/behaviour/timing';
import { cleanupTabRuntime } from '@workbench/state/tabRuntime';
import { loadTerminalRenderer } from '@workbench/terminal/terminalRendererLoader';
import { storeToRefs } from 'pinia';
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import PermissionHint from '../components/PermissionHint.vue';
import { useAuthStore } from '../state/auth';
import KeyBar from '../terminal/KeyBar.vue';
import { useRemoteTerminal } from '../terminal/useRemoteTerminal';

// Full-screen control of one agent's Claude Code session. The computer's window keeps its own
// view behind an overlay until it takes the terminal back.
const props = defineProps<{ sessionId: string }>();

const FONT_MIN = 8;
const FONT_MAX = 20;
const STATUS_LABEL = {
  connecting: 'common.connecting.loading',
  live: 'mobile.terminal.status.live.status',
  reconnecting: 'mobile.terminal.status.reconnecting.status',
  ended: 'mobile.terminal.status.ended.status',
} as const;

type Renderer = Awaited<ReturnType<typeof loadTerminalRenderer>>;
type Attached = ReturnType<Renderer['getOrCreateTerminal']>;

const router = useRouter();
const { permissions } = storeToRefs(useAuthStore());
const container = ref<HTMLElement | null>(null);
const fontSize = usePersistedState('kira.mobileTerminal.fontSize', 12);
const compose = ref('');
const ctrl = ref(false);
const appCursor = ref(false);
const viewportHeight = ref<number | null>(null);
let renderer: Renderer | null = null;
let attached: Attached | null = null;

const remote = useRemoteTerminal(props.sessionId, {
  size: () => ({ cols: attached?.term.cols ?? 80, rows: attached?.term.rows ?? 24 }),
  write: (bytes) => attached?.term.write(bytes),
  reset: () => attached?.term.reset(),
});

// Sticky Ctrl turns the next typed letter into its control byte, then clears itself.
function fromTerminal(bytes: Uint8Array): void {
  const first = bytes[0];
  if (ctrl.value && bytes.length === 1 && first !== undefined) {
    ctrl.value = false;
    const letter = first | 0x20;
    if (letter >= 0x61 && letter <= 0x7a) {
      remote.sendInput(Uint8Array.of(first & 0x1f));
      return;
    }
  }
  remote.sendInput(bytes);
}

const refit = useDebounceFn(() => {
  const dims = renderer?.fitTerminal(props.sessionId);
  if (dims) remote.sendResize(dims.cols, dims.rows);
}, TIMING.search);

onMounted(async () => {
  if (!permissions.value.agentInput || !container.value) return;
  renderer = await loadTerminalRenderer();
  attached = renderer.getOrCreateTerminal(props.sessionId, {
    appearance: () => ({
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      fontSize: fontSize.value,
    }),
    onTerminalOutput: () => () => {},
    writeTerminal: (_id, bytes) => fromTerminal(bytes),
    openLink: (url) => void window.open(url, '_blank', 'noopener,noreferrer'),
    linkActivation: 'tap',
  });
  attached.term.onWriteParsed(() => {
    appCursor.value = attached?.term.modes.applicationCursorKeysMode ?? false;
  });
  container.value.append(attached.host);
  await nextTick();
  renderer.fitTerminal(props.sessionId);
  remote.connect();
});

onBeforeUnmount(() => {
  remote.release();
  cleanupTabRuntime(props.sessionId);
  attached = null;
});

watch(fontSize, (n) => {
  if (!attached) return;
  attached.term.options.fontSize = n;
  void refit();
});
useResizeObserver(container, () => void refit());
useEventListener(
  () => window.visualViewport,
  'resize',
  () => {
    viewportHeight.value = window.visualViewport?.height ?? null;
    void refit();
  },
);

function step(delta: number): void {
  fontSize.value = Math.min(FONT_MAX, Math.max(FONT_MIN, fontSize.value + delta));
}

function sendCompose(): void {
  const text = compose.value;
  if (!text) return;
  attached?.term.paste(text);
  remote.sendText('\r');
  compose.value = '';
}

function leave(): void {
  remote.release();
  if (window.history.length > 1) router.back();
  else void router.replace('/needs');
}
</script>

<template>
  <section
    class="flex h-full flex-col bg-bg pt-[env(safe-area-inset-top)]"
    :style="viewportHeight ? { height: `${viewportHeight}px` } : undefined"
    data-testid="terminal-screen"
  >
    <header class="flex items-center gap-2 border-b border-border bg-chrome px-2 py-1">
      <Button
        size="touch-icon"
        variant="toolbar"
        :aria-label="t('common.action.back.label')"
        data-testid="term-back"
        @click="leave"
      >
        <CodiconIcon name="chevron-left" :size="16" />
      </Button>
      <h1 class="m-0 min-w-0 flex-1 truncate text-kira-lg font-medium">{{
        t('common.module.terminal.label')
      }}</h1>
      <span class="text-kira-sm text-muted-foreground" role="status" data-testid="term-status">
        {{ t(STATUS_LABEL[remote.status.value]) }}
      </span>
      <Button
        size="touch-icon"
        variant="toolbar"
        :aria-label="t('mobile.terminal.fontDown.aria')"
        data-testid="term-font-down"
        :disabled="fontSize <= FONT_MIN"
        @click="step(-1)"
        >{{
          t('mobile.terminal.fontDown.label')
        }}</Button
      >
      <Button
        size="touch-icon"
        variant="toolbar"
        :aria-label="t('mobile.terminal.fontUp.aria')"
        data-testid="term-font-up"
        :disabled="fontSize >= FONT_MAX"
        @click="step(1)"
        >{{
          t('mobile.terminal.fontUp.label')
        }}</Button
      >
      <Button size="touch" variant="dialog" data-testid="term-release" @click="leave">{{
        t('mobile.terminal.release.label')
      }}</Button>
    </header>

    <PermissionHint v-if="!permissions.agentInput" :what="t('mobile.terminal.controlOff.hint')" />
    <template v-else>
      <Alert v-if="remote.status.value === 'ended'" variant="info" strip data-testid="term-ended">
        <AlertDescription>{{ remote.endReason.value }}</AlertDescription>
      </Alert>
      <p
        v-if="remote.notice.value"
        class="m-0 px-3 py-1 text-kira-sm text-warn"
        data-testid="term-notice"
        >{{
          remote.notice.value
        }}</p
      >
      <div ref="container" class="min-h-0 flex-1 overflow-hidden" data-testid="term-host" />
      <KeyBar v-model:ctrl="ctrl" :app-cursor="appCursor" @key="(seq) => remote.sendText(seq)" />
      <form
        class="flex gap-2 px-2 pb-[max(0.5rem,env(safe-area-inset-bottom))]"
        @submit.prevent="sendCompose"
      >
        <Input
          :model-value="compose"
          size="touch"
          class="flex-1"
          :placeholder="t('mobile.terminal.compose.placeholder')"
          enterkeyhint="send"
          autocapitalize="off"
          autocomplete="off"
          data-testid="term-compose"
          @update:model-value="(v) => (compose = String(v))"
        />
        <Button
          size="touch"
          type="submit"
          variant="dialog-primary"
          :disabled="!compose"
          data-testid="term-compose-send"
          >{{
            t('common.action.send.label')
          }}</Button
        >
      </form>
    </template>
  </section>
</template>
