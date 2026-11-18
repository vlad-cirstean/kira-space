<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import Spinner from '@theme/components/Spinner.vue';
import StatusDot from '@theme/components/StatusDot.vue';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { rowActions, rowIndent, rowMeta, rowVariants } from '@theme/lib/rowVariants';
import { TONE_DOT } from '@theme/lib/statusTone';
import { cn } from '@theme/lib/utils';
import { computed } from 'vue';
import { containerStateTone, publishedPorts } from '../lib/format';
import type { ContainerAction } from '../queries';
import type { DockerContainer } from '../wire';
import OriginIcon from './OriginIcon.vue';

const props = defineProps<{
  container: DockerContainer;
  grouped: boolean;
  selected: boolean;
  busy: boolean;
}>();

const emit = defineEmits<{
  select: [DockerContainer];
  open: [DockerContainer];
  act: [DockerContainer, ContainerAction];
}>();

const running = computed(() => props.container.state === 'running');
const rowClass = computed(() =>
  cn(
    rowVariants({ layout: 'double', selected: props.selected }),
    'group/row grid h-full grid-cols-[0.5rem_minmax(0,1fr)_auto_1.25rem] grid-rows-[auto_auto] items-center gap-x-1.5 gap-y-0.5',
  ),
);
const indent = computed(() => rowIndent(props.grouped ? 1 : 0));
const portLabel = computed(() => {
  const ports = publishedPorts(props.container.ports);
  return ports.length > 1 ? `${ports[0]} +${ports.length - 1}` : (ports[0] ?? '');
});
</script>

<template>
  <!-- biome-ignore lint/a11y/useFocusableInteractive: tabindex falls through from the list's `nav`. -->
  <!-- biome-ignore lint/a11y/useKeyWithClickEvents: Enter comes from useListNavigation on the container. -->
  <div
    :class="rowClass"
    :style="indent"
    data-testid="docker-row"
    :data-id="container.id"
    :data-name="container.name"
    :data-state="container.state"
    role="option"
    :aria-selected="selected"
    @click="emit('select', container)"
    @dblclick="emit('open', container)"
  >
    <StatusDot
      :tone="TONE_DOT[containerStateTone(container.state)]"
      data-testid="docker-state-dot"
    />
    <span class="flex min-w-0 items-center gap-1.5">
      <span
        class="truncate"
        :class="running ? '' : 'text-muted-foreground'"
        data-testid="docker-row-name"
        >{{
          container.name
        }}</span
      >
      <OriginIcon
        v-if="container.origin !== '' && !(container.origin === 'compose' && grouped)"
        tip="attribute"
        :origin="container.origin"
        :name="container.originName"
      />
    </span>
    <span v-if="!running" :class="cn('text-right', rowMeta)" data-testid="docker-row-state">{{
      container.state
    }}</span>
    <span v-else />
    <span class="row-span-2 flex items-center justify-end">
      <Spinner v-if="busy" label="" />
      <span v-else :class="cn('flex', rowActions)">
        <Button
          v-if="!running"
          variant="toolbar"
          size="kira-icon"
          :aria-label="t('common.action.start.label')"
          :data-kira-tip="t('common.action.start.label')"
          data-tone="ok"
          data-testid="docker-row-start"
          @click.stop="emit('act', container, 'start')"
        >
          <CodiconIcon name="play" :size="13" />
        </Button>
        <Button
          v-else
          variant="toolbar"
          size="kira-icon"
          :aria-label="t('common.action.stop.label')"
          :data-kira-tip="t('common.action.stop.label')"
          data-tone="error"
          data-testid="docker-row-stop"
          @click.stop="emit('act', container, 'stop')"
        >
          <CodiconIcon name="debug-stop" :size="13" />
        </Button>
      </span>
    </span>
    <span />
    <span class="col-span-2 flex min-w-0 items-baseline gap-1.5 text-kira-sm text-muted-foreground">
      <span class="truncate" data-testid="docker-row-image">{{ container.image }}</span>
      <span
        v-if="portLabel"
        class="max-w-[40%] shrink-0 truncate font-data"
        data-testid="docker-row-ports"
        >{{
          portLabel
        }}</span
      >
    </span>
  </div>
</template>
