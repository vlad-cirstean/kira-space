<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Button } from '@theme/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@theme/components/ui/dialog';
import { FieldDescription } from '@theme/components/ui/field';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { formatClockTime } from '@theme/format';
import { t } from '@theme/i18n';
import { useCopy } from '@workbench/behaviour/clipboard';
import { computed, ref } from 'vue';
import { useRunSandbox, useSessionSandbox } from '../queries';
import type { RunSandboxBadge } from '../wire';

// The run's sandbox: a lock chip (access, blocked count) that opens the policy, the first denials
// and the profile. Detail loads when the popover opens. A manual step session passes sessionId.
const props = defineProps<{ runId?: string; sessionId?: string; badge: RunSandboxBadge }>();
const open = ref(false);
const profileOpen = ref(false);
const shown = (): boolean => open.value || profileOpen.value;
const runDetail = useRunSandbox(() => props.runId ?? '', shown);
const sessionDetail = useSessionSandbox(() => props.sessionId ?? '', shown);
const detail = computed(() => (props.sessionId ? sessionDetail : runDetail));
const { copy, copied } = useCopy();

const label = computed(() => {
  const b = props.badge;
  if (b.denials > 0) return t('space.ade.sandbox.blocked.label', { count: b.denials });
  if (!b.enforced) return t('space.ade.sandbox.notEnforced.label');
  return t(
    b.access === 'off'
      ? 'space.ade.sandbox.off.label'
      : b.access === 'read'
        ? 'space.ade.sandbox.read.label'
        : 'space.ade.sandbox.write.label',
  );
});
const tone = computed(() => {
  const b = props.badge;
  if (b.denials > 0 || b.access === 'off') return 'text-warn-text';
  return b.enforced ? 'text-muted-foreground' : 'text-subtle';
});
const d = computed(() => detail.value.data.value ?? null);
const lists = computed(() =>
  d.value
    ? (
        [
          ['write', 'space.ade.sandbox.popover.write.label', d.value.write],
          ['read', 'space.ade.sandbox.popover.read.label', d.value.read],
          ['allow', 'space.ade.sandbox.popover.allow.label', d.value.allow],
        ] as const
      ).filter(([, , paths]) => paths.length > 0)
    : [],
);
</script>

<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        variant="toolbar"
        size="kira"
        class="shrink-0 gap-1"
        :class="tone"
        :data-testid="sessionId ? 'ade-session-sandbox-badge' : 'ade-run-sandbox'"
        :data-access="badge.access"
        :data-enforced="badge.enforced"
        :data-denials="badge.denials"
      >
        <CodiconIcon name="lock" :size="12" />{{ label }}
      </Button>
    </PopoverTrigger>
    <PopoverContent class="w-96 text-kira-md" align="start" data-testid="ade-run-sandbox-popover">
      <div v-if="d" class="flex flex-col gap-2">
        <FieldDescription data-testid="ade-run-sandbox-enforced">{{
          d.enforced ? t('space.ade.sandbox.popover.enforced.status') : d.reason
        }}</FieldDescription>
        <div
          v-for="[ id, key, paths ] in lists"
          :key="id"
          class="flex flex-col gap-0.5"
          :data-testid="`ade-run-sandbox-${id}`"
        >
          <span class="text-kira-sm text-muted-foreground">{{ t(key) }}</span>
          <span v-for="p in paths" :key="p" class="truncate font-data text-kira-sm">{{ p }}</span>
        </div>
        <div class="flex gap-3 text-kira-sm">
          <span class="text-muted-foreground">{{
            t('space.ade.sandbox.popover.network.label')
          }}</span>
          <span data-testid="ade-run-sandbox-network">{{
            d.network
              ? t('space.ade.sandbox.popover.networkOn.status')
              : t('space.ade.sandbox.popover.networkOff.status')
          }}</span>
        </div>
        <div class="flex flex-col gap-0.5" data-testid="ade-run-sandbox-denials">
          <span class="text-kira-sm text-muted-foreground">{{
            t('space.ade.sandbox.popover.denials.label')
          }}</span>
          <span v-if="d.denials.length === 0" class="text-kira-sm text-subtle">{{
            t('common.state.none.option')
          }}</span>
          <div
            v-for="(x, i) in d.denials"
            :key="i"
            class="flex gap-2 font-data text-kira-sm"
            data-testid="ade-run-sandbox-denial"
          >
            <span class="shrink-0 text-subtle">{{ formatClockTime(x.at) }}</span>
            <span class="shrink-0 text-warn-text">{{ x.op }}</span>
            <span class="min-w-0 flex-1 truncate">{{ x.target }}</span>
            <span class="shrink-0 text-subtle">{{ x.process }}</span>
          </div>
          <span v-if="d.denialCount > d.denials.length" class="text-kira-sm text-subtle">{{
            t('space.ade.sandbox.popover.shown.status', {
              shown: d.denials.length,
              total: d.denialCount,
            })
          }}</span>
        </div>
        <Button
          v-if="d.profile"
          variant="dialog"
          size="kira"
          class="self-start"
          data-testid="ade-run-sandbox-profile"
          @click="profileOpen = true"
        >
          {{ t('space.ade.sandbox.popover.viewProfile.label') }}
        </Button>
      </div>
    </PopoverContent>
  </Popover>
  <Dialog v-model:open="profileOpen">
    <DialogContent size="lg" data-testid="ade-run-sandbox-profile-dialog">
      <DialogHeader>
        <DialogTitle>{{ t('space.ade.sandbox.profile.title') }}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <Button
          variant="toolbar"
          size="kira"
          class="self-start"
          data-testid="ade-run-sandbox-profile-copy"
          @click="copy(d?.profile ?? '')"
        >
          {{ copied ? t('common.copy.copied.announce') : t('common.action.copy.label') }}
        </Button>
        <CodeBlock class="max-h-96 overflow-auto" data-testid="ade-run-sandbox-profile-text">{{
          d?.profile ?? ''
        }}</CodeBlock>
      </DialogBody>
    </DialogContent>
  </Dialog>
</template>
