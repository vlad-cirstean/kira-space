<script setup lang="ts">
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { t } from '@theme/i18n';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../../../bridge/control';
import { useOpenHostView } from '../../../hostview/queries';
import AdeChip from '../AdeChip.vue';
import { artifactKindLabel, artifactStatusLabel, artifactStatusTone } from '../artifacts/status';
import type { Artifact } from '../wire';
import AdeDetailsLinkEdit from './AdeDetailsLinkEdit.vue';
import AdeLinkKindMenu from './AdeLinkKindMenu.vue';
import type { LinkItem } from './linkGroups';

// One link row: kind, title as a link, status, then edit and remove. Detected rows are read-only; a
// stored row's kind chip is a menu.
const props = defineProps<{
  item: LinkItem;
  editing: boolean;
  readonly?: boolean;
  commit: (item: LinkItem, text: string) => Promise<void>;
}>();
const emit = defineEmits<{ edit: []; done: []; remove: []; kind: [kind: Artifact['kind']] }>();

const openView = useOpenHostView();
const canEdit = () => !props.readonly && props.item.origin !== 'detected';

function open(e: MouseEvent): void {
  e.preventDefault();
  const { kind, url } = props.item;
  if (kind === 'changeRequest') fireAndForget(openView.mutateAsync(url), 'hostView.open');
  else fireAndForget(control.openExternal(url), 'ade.details');
}
</script>

<template>
  <li
    class="flex min-h-row min-w-0 items-center gap-2 text-kira-md"
    data-testid="ade-details-link"
    :data-origin="item.origin"
    :data-url="item.url"
    @dblclick="canEdit() && !editing && emit('edit')"
    @keydown.f2.prevent="canEdit() && !editing && emit('edit')"
  >
    <AdeDetailsLinkEdit
      v-if="editing"
      :text="item.editText"
      :commit="(text) => commit(item, text)"
      @done="emit('done')"
    />
    <template v-else>
      <AdeLinkKindMenu
        v-if="item.origin === 'stored' && canEdit()"
        :kind="item.kind"
        @pick="emit('kind', $event)"
      />
      <AdeChip v-else :label="artifactKindLabel(item.kind)" tone="grey" wide />
      <a
        v-if="item.url"
        :href="item.url"
        class="min-w-0 truncate text-info"
        data-testid="ade-details-link-title"
        @click="open"
        >{{
          item.label
        }}</a
      >
      <span v-else class="min-w-0 truncate" data-testid="ade-details-link-title">{{
        item.label
      }}</span>
      <AdeChip
        v-if="item.status"
        :label="artifactStatusLabel(item.status)"
        :tone="artifactStatusTone(item.status)"
      />
      <span class="flex-1" />
      <slot name="trailing" />
      <template v-if="canEdit()">
        <TooltipIconButton
          icon="edit"
          :label="t('common.action.edit.label')"
          class="shrink-0"
          data-testid="ade-details-link-edit"
          @click="emit('edit')"
        />
        <TooltipIconButton
          icon="close"
          :label="t('common.action.remove.label')"
          class="shrink-0"
          data-testid="ade-details-link-remove"
          @click="emit('remove')"
        />
      </template>
    </template>
  </li>
</template>
