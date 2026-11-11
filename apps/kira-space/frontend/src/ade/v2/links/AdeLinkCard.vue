<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { t } from '@theme/i18n';
import { isPrimaryModifier } from '@workbench/shortcuts/keys';
import AdeChip from '../AdeChip.vue';
import {
  artifactKindLabel,
  artifactStatusLabel,
  artifactStatusTone,
  kindIcon,
} from '../artifacts/status';
import AdeLinkOpenExternal from '../details/AdeLinkOpenExternal.vue';
import AdeOriginSession from '../details/AdeOriginSession.vue';
import AdeTaskIssueLine from '../panel/AdeTaskIssueLine.vue';
import type { Artifact, Task } from '../wire';
import AdeLinkEdit from './AdeLinkEdit.vue';
import { KINDS, type LinkItem } from './linkGroups';
import { sourceLabel } from './sourceLabel';
import type { TaskLinks } from './useTaskLinks';

// The open card of one pill: full title, url, status, repo, source, kind toggles, open, session, edit and
// remove. Edit mode shows the kind toggles and the actions that change the link; detected links never edit.
const props = defineProps<{
  task: Task;
  item: LinkItem;
  mode: 'edit' | 'view' | 'add';
  archived?: boolean;
  editing: boolean;
  links: TaskLinks;
}>();
const emit = defineEmits<{ edit: []; done: []; openSession: [sessionId: string] }>();

const canEdit = () => props.mode === 'edit' && props.item.origin !== 'detected';
const kindItems = KINDS.map((k) => ({
  value: k,
  label: artifactKindLabel(k),
  icon: kindIcon(k),
  tooltip: artifactKindLabel(k),
  testid: `ade-links-card-kind-${k}`,
}));

function onKind(value: string): void {
  void props.links.recategorise(props.item, value as Artifact['kind']);
}
// The title opens like its pill: primary-modifier click opens the browser.
function openTitle(e: MouseEvent): void {
  e.preventDefault();
  props.links.openLink(props.item, { external: isPrimaryModifier(e) });
}
</script>

<template>
  <div class="flex flex-col gap-1.5" data-testid="ade-links-card-body">
    <AdeLinkEdit
      v-if="editing"
      :text="item.editText"
      :commit="(text) => links.commit(item, text)"
      @keydown.escape.stop
      @done="emit('done')"
    />
    <div v-else class="flex min-w-0 items-start gap-1.5">
      <a
        v-if="item.url"
        :href="item.url"
        class="line-clamp-2 min-w-0 flex-1 break-words font-medium text-info"
        data-testid="ade-links-card-title"
        @click="openTitle"
        >{{
          item.title || item.label
        }}</a
      >
      <span
        v-else
        class="line-clamp-2 min-w-0 flex-1 break-words font-medium"
        data-testid="ade-links-card-title"
        >{{
          item.title || item.label
        }}</span
      >
      <AdeChip
        v-if="item.status && item.origin !== 'jira'"
        :label="artifactStatusLabel(item.status)"
        :tone="artifactStatusTone(item.status)"
      />
    </div>
    <div
      v-if="item.url"
      class="font-data text-kira-sm text-subtle"
      data-testid="ade-links-card-url"
    >
      <TruncatedText :text="item.url" />
    </div>
    <div
      v-if="item.repo || item.source || ((mode === 'edit' || archived) && item.sessionId)"
      class="flex items-center gap-1.5 text-kira-sm text-muted-foreground"
    >
      <span v-if="item.repo" class="min-w-0 truncate" data-testid="ade-links-card-repo">{{
        item.repo
      }}</span>
      <span v-if="item.source" data-testid="ade-links-card-source">{{
        sourceLabel(item.source)
      }}</span>
      <AdeOriginSession
        v-if="mode === 'edit' || archived"
        :task-id="task.id"
        :session-id="item.sessionId"
        :archived="archived"
        @open="emit('openSession', $event)"
      />
    </div>
    <AdeTaskIssueLine
      v-if="item.origin === 'jira'"
      :task-id="task.id"
      :link="`${item.label} ${item.url}`"
    />
    <SecondaryTabs
      v-if="canEdit() && item.origin === 'stored'"
      :model-value="item.kind"
      :items="kindItems"
      variant="segmented"
      :aria-label="t('space.ade.links.kind.aria')"
      data-testid="ade-links-card-kind"
      @update:model-value="onKind"
    >
      <template #item="{ item: k }">
        <CodiconIcon :name="k.icon ?? ''" :size="13" :aria-label="k.label" role="img" />
      </template>
    </SecondaryTabs>
    <div v-if="!editing" class="flex gap-1 border-t border-border pt-1.5">
      <AdeLinkOpenExternal :url="item.url" />
      <template v-if="canEdit()">
        <TooltipIconButton
          icon="edit"
          :label="t('common.action.edit.label')"
          class="shrink-0"
          data-testid="ade-links-card-edit"
          @click="emit('edit')"
        />
        <TooltipIconButton
          icon="close"
          :label="t('common.action.remove.label')"
          class="shrink-0"
          data-testid="ade-links-card-remove"
          @click="links.remove(item)"
        />
      </template>
    </div>
  </div>
</template>
