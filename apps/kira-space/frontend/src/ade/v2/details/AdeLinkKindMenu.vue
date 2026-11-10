<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@theme/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import type { MessageKey } from '@theme/i18n/types';
import { artifactKindLabel, kindIcon } from '../artifacts/status';
import type { Artifact } from '../wire';
import { LINK_CATEGORIES, type LinkCategory, linkCategory } from './linkGroups';

// The kind icon of a stored row, as a menu; picking another kind re-files the row.
defineProps<{ kind: Artifact['kind'] }>();
const emit = defineEmits<{ pick: [kind: Artifact['kind']] }>();

const KINDS: Artifact['kind'][] = ['issue', 'changeRequest', 'branch', 'deployment', 'doc', 'link'];
const GROUP_LABEL = {
  issue: 'space.settings.updates.issue.title',
  changeRequest: 'space.ade.details.group.changeRequest.label',
  misc: 'space.ade.details.group.misc.label',
} as const satisfies Record<LinkCategory, MessageKey>;
</script>

<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <span class="inline-flex">
        <DropdownMenu :modal="false">
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              class="flex size-5 shrink-0 items-center justify-center rounded-kira-sm text-muted-foreground hover:bg-field focus-visible:focus-ring"
              :aria-label="
                t('space.ade.details.kindMenu.tooltip', { kind: artifactKindLabel(kind) })
              "
              :data-kind="kind"
              data-testid="ade-details-link-kind"
            >
              <CodiconIcon :name="kindIcon(kind)" :size="13" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuRadioGroup
              :model-value="kind"
              @update:model-value="emit('pick', $event as Artifact['kind'])"
            >
              <template v-for="(category, i) in LINK_CATEGORIES" :key="category">
                <DropdownMenuSeparator v-if="i > 0" />
                <DropdownMenuLabel>{{ t(GROUP_LABEL[category]) }}</DropdownMenuLabel>
                <DropdownMenuRadioItem
                  v-for="k in KINDS.filter((x) => linkCategory(x) === category)"
                  :key="k"
                  :value="k"
                  :data-kind="k"
                  data-testid="ade-details-link-kind-item"
                >
                  <CodiconIcon :name="kindIcon(k)" :size="13" class="mr-1.5" />
                  {{ artifactKindLabel(k) }}
                  <template #indicator-icon><CodiconIcon name="check" :size="13" /></template>
                </DropdownMenuRadioItem>
              </template>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    </TooltipTrigger>
    <TooltipContent>{{
      t('space.ade.details.kindMenu.tooltip', { kind: artifactKindLabel(kind) })
    }}</TooltipContent>
  </Tooltip>
</template>
