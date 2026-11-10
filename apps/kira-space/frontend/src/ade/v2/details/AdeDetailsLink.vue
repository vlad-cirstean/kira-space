<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { rowActions } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { control } from '../../../bridge/control';
import { useOpenChangeRequest } from '../../../hostview/queries';
import AdeChip from '../AdeChip.vue';
import {
  artifactKindLabel,
  artifactStatusLabel,
  artifactStatusTone,
  kindIcon,
} from '../artifacts/status';
import AdeRow from '../panel/AdeRow.vue';
import type { Artifact } from '../wire';
import AdeDetailsLinkEdit from './AdeDetailsLinkEdit.vue';
import AdeLinkKindMenu from './AdeLinkKindMenu.vue';
import AdeLinkOpenExternal from './AdeLinkOpenExternal.vue';
import type { LinkItem } from './linkGroups';

// One link row: group name (first row of a group), kind icon, title as a link, status, then open in browser, edit and
// remove on hover. Detected rows are read-only; a stored row's kind icon is a menu.
const props = defineProps<{
  item: LinkItem;
  /** The group's name; '' on every row but the group's first. */
  groupLabel: string;
  editing: boolean;
  readonly?: boolean;
  commit: (item: LinkItem, text: string) => Promise<void>;
}>();
const emit = defineEmits<{ edit: []; done: []; remove: []; kind: [kind: Artifact['kind']] }>();

const openView = useOpenChangeRequest();
const canEdit = () => !props.readonly && props.item.origin !== 'detected';

function open(e: MouseEvent): void {
  e.preventDefault();
  const { kind, url } = props.item;
  if (kind === 'changeRequest') fireAndForget(openView.mutateAsync(url), 'hostView.open');
  else fireAndForget(control.openExternal(url), 'ade.details');
}
</script>

<template>
  <AdeRow
    as="li"
    label=""
    class="group"
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
      <a
        v-if="item.url"
        :href="item.url"
        :class="cn('min-w-0 text-info', item.origin === 'jira' ? 'shrink-0' : 'truncate')"
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
      <slot name="extra" />
    </template>
    <template #lead>
      <AdeLinkKindMenu
        v-if="item.origin === 'stored' && canEdit()"
        :kind="item.kind"
        @pick="emit('kind', $event)"
      />
      <Tooltip v-else>
        <TooltipTrigger as-child>
          <span
            class="inline-flex text-muted-foreground"
            :data-kind="item.kind"
            :aria-label="artifactKindLabel(item.kind)"
            data-testid="ade-details-link-icon"
            ><CodiconIcon :name="kindIcon(item.kind)" :size="13" /></span
          >
        </TooltipTrigger>
        <TooltipContent>{{ artifactKindLabel(item.kind) }}</TooltipContent>
      </Tooltip>
    </template>
    <template #label>
      <span class="truncate" data-testid="ade-details-group-label">{{ groupLabel }}</span>
    </template>
    <template v-if="!editing" #actions>
      <span :class="cn('flex items-center gap-1', rowActions)">
        <AdeLinkOpenExternal :url="item.url" />
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
      </span>
    </template>
  </AdeRow>
</template>
