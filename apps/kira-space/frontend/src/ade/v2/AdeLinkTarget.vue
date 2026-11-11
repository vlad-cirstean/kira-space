<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import BranchChip from '@theme/components/BranchChip.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldError } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useRepoLinksStore } from '../../repo/state/repoLinks';
import { repoText } from '../../state/coderepos';
import { artifactStatusTone } from './artifacts/status';
import AdeLinkPill from './links/AdeLinkPill.vue';
import type { LinkItem } from './links/linkGroups';
import type { HostLinkTarget } from './wire';

const props = defineProps<{
  target: HostLinkTarget | undefined;
  repo: string;
  pending: boolean;
  error: string;
  busy: boolean;
  attach: boolean;
}>();
const emit = defineEmits<{ add: [] }>();

const repoLinks = useRepoLinksStore();
const repoTone = computed(() => repoText(repoLinks.repoColorOf(props.target?.codeRepoId ?? '')));

// The pasted target as the pill the strip will show once added.
const item = computed<LinkItem | null>(() => {
  const tg = props.target;
  if (!tg) return null;
  const cr = tg.changeRequest;
  const status = cr ? cr.state.toLowerCase() : '';
  return {
    key: cr ? cr.url : tg.branch,
    label: cr ? `#${cr.number} ${cr.title}` : tg.branch,
    title: cr ? cr.title : tg.branch,
    repo: props.repo,
    source: 'host',
    editText: cr ? cr.url : tg.branch,
    url: cr ? cr.url : '',
    ref: '',
    kind: cr ? 'changeRequest' : 'branch',
    origin: 'host',
    sessionId: '',
    status: status || undefined,
    tone: status ? artifactStatusTone(status) : undefined,
  };
});
</script>

<template>
  <div class="flex flex-col gap-2 p-2 text-kira-md">
    <span v-if="pending" class="text-muted-foreground" data-testid="ade-link-pending">{{
      t('space.ade.add.link.pending.label')
    }}</span>
    <FieldError v-else-if="error" data-testid="ade-link-error">{{ error }}</FieldError>
    <template v-else-if="target && item">
      <div class="flex min-w-0 flex-wrap items-center gap-1.5" data-testid="ade-link-target">
        <CodiconIcon
          :name="target.changeRequest ? 'git-pull-request' : 'git-branch'"
          :size="12"
          class="shrink-0 text-muted-foreground"
        />
        <span
          class="shrink-0 text-kira-sm font-medium"
          :class="repoTone.class"
          :style="repoTone.style"
          >{{
            repo
          }}</span
        >
        <AdeLinkPill :item="item" as="span" show-kind data-testid="ade-link-target-pill" />
        <BranchChip v-if="target.changeRequest" :name="target.branch" class="max-w-60" />
        <Badge v-if="target.fork" variant="chip" data-testid="ade-link-fork">{{
          t('space.ade.add.link.fork.label', { owner: target.headOwner })
        }}</Badge>
        <span v-if="target.author" class="text-kira-sm text-muted-foreground">{{
          t('space.ade.add.link.author.label', { author: target.author })
        }}</span>
        <Button
          class="ml-auto"
          variant="dialog-primary"
          size="kira-lg"
          :pending="busy"
          :disabled="target.onTaskId !== ''"
          data-testid="ade-link-add"
          @click="emit('add')"
        >
          {{ attach ? t('docker.edit.attach.label') : t('common.action.add.label') }}
        </Button>
      </div>
      <span v-if="target.onTaskId" class="text-warn" data-testid="ade-link-on-task">{{
        t('space.ade.add.link.onTask.label')
      }}</span>
    </template>
  </div>
</template>
