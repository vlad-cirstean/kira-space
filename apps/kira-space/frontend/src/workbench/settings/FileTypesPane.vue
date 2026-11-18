<script setup lang="ts">
import type { FileHandlerExt, FileHandlerGroup } from '@shared/domain/repo';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import Disclosure from '@theme/components/Disclosure.vue';
import { Alert } from '@theme/components/ui/alert';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@theme/components/ui/field';
import { Switch } from '@theme/components/ui/switch';
import { t } from '@theme/i18n';
import { errorText } from '@workbench/bridge/codedError';
import { computed, ref, useId, watch } from 'vue';
import { control } from '../../bridge/control';
import type { SettingsPaneProps } from './types';

// LaunchServices is the source of truth and Finder can change a default while this dialog is
// closed, so the pane shows live state, re-read each time it opens, never a stored boolean. Like
// MobileAccessPane it bypasses draft/Save: a switch is an action on the OS, not a settings leaf.
const props = defineProps<SettingsPaneProps>();

const FILE_HANDLERS_KEY = ['fileHandlers'] as const;
const queryClient = useQueryClient();
const baseId = useId();

const status = useQuery({
  queryKey: FILE_HANDLERS_KEY,
  queryFn: () => control.fileHandlersStatus(),
  enabled: computed(() => props.active),
});
watch(
  () => props.active,
  (active) => {
    if (active && status.data.value) void status.refetch();
  },
);

const unrestored = ref<string[]>([]);
const setHandlers = useMutation({
  mutationFn: (v: { exts: string[]; on: boolean }) => control.fileHandlersSet(v.exts, v.on),
  onSuccess: (res) => {
    unrestored.value = res.unrestored;
  },
  onSettled: () => queryClient.invalidateQueries({ queryKey: FILE_HANDLERS_KEY }),
});

const supported = computed(() => status.data.value?.supported ?? true);
const groups = computed(() => status.data.value?.groups ?? []);
const allUnavailable = computed(() => {
  const rows = groups.value.flatMap((g) => g.extensions);
  return rows.length > 0 && rows.every((e) => e.state === 'unavailable');
});

const groupLabel = (id: string): string =>
  id === 'code'
    ? t('space.settings.fileTypes.group.code.label')
    : t('space.settings.fileTypes.group.data.label');

function available(group: FileHandlerGroup): FileHandlerExt[] {
  return group.extensions.filter((e) => e.state !== 'unavailable');
}
function selfCount(group: FileHandlerGroup): number {
  return group.extensions.filter((e) => e.state === 'self').length;
}
function groupOn(group: FileHandlerGroup): boolean {
  const rows = available(group);
  return rows.length > 0 && selfCount(group) === rows.length;
}
function groupMixed(group: FileHandlerGroup): boolean {
  return selfCount(group) > 0 && !groupOn(group);
}

function toggleGroup(group: FileHandlerGroup, on: boolean): void {
  setHandlers.mutate({ exts: available(group).map((e) => e.ext), on });
}
function toggleExt(ext: FileHandlerExt, on: boolean): void {
  setHandlers.mutate({ exts: [ext.ext], on });
}

function handlerText(ext: FileHandlerExt): string {
  if (ext.state === 'unavailable') return t('space.settings.fileTypes.handler.unavailable.label');
  if (ext.state === 'none' || !ext.handler) return t('common.state.none.option');
  return ext.handler.name;
}

const open = ref<Record<string, boolean>>({});
</script>

<template>
  <div class="contents" v-show="active">
    <FieldDescription>{{ t('space.settings.fileTypes.description') }}</FieldDescription>

    <Alert v-if="!supported" variant="info" data-testid="file-types-unsupported">
      {{ t('space.settings.fileTypes.unsupported.body') }}
    </Alert>
    <template v-else>
      <Alert v-if="allUnavailable" variant="warn" data-testid="file-types-unregistered">
        {{ t('space.settings.fileTypes.unregistered.body') }}
      </Alert>
      <Alert v-if="unrestored.length > 0" variant="warn" data-testid="file-types-unrestored">
        {{ t('space.settings.fileTypes.unrestored.body', { exts: unrestored.join(', ') }) }}
      </Alert>
      <FieldError v-if="status.error.value" data-testid="file-types-load-error">
        {{ errorText(status.error.value) }}
      </FieldError>
      <FieldError v-if="setHandlers.error.value" data-testid="file-types-set-error">
        {{ errorText(setHandlers.error.value) }}
      </FieldError>

      <Field v-for="group in groups" :key="group.id" :data-testid="`file-types-group-${group.id}`">
        <Field orientation="horizontal">
          <Switch
            :id="`${baseId}-${group.id}`"
            :model-value="groupOn(group)"
            :aria-checked="groupMixed(group) ? 'mixed' : groupOn(group)"
            :disabled="setHandlers.isPending.value || available(group).length === 0"
            :data-testid="`file-types-switch-${group.id}`"
            :data-mixed="groupMixed(group)"
            @update:model-value="(v) => toggleGroup(group, v === true)"
          />
          <FieldContent>
            <FieldLabel :for="`${baseId}-${group.id}`">{{ groupLabel(group.id) }}</FieldLabel>
            <FieldDescription v-if="groupMixed(group)" :data-testid="`file-types-some-${group.id}`">
              {{ t('space.settings.fileTypes.some.status', { count: selfCount(group) }) }}
            </FieldDescription>
          </FieldContent>
        </Field>
        <Disclosure
          v-model:open="open[group.id]"
          :label="
            t('space.settings.fileTypes.extensions.label', { count: group.extensions.length })
          "
          :data-testid="`file-types-disclosure-${group.id}`"
          content-class="flex flex-col gap-1 pt-1"
        >
          <div
            v-for="ext in group.extensions"
            :key="ext.ext"
            class="flex items-center gap-2 text-kira-sm"
            :data-testid="`file-types-row-${ext.ext}`"
            :data-state="ext.state"
          >
            <Switch
              :model-value="ext.state === 'self'"
              :disabled="setHandlers.isPending.value || ext.state === 'unavailable'"
              :aria-label="t('space.settings.fileTypes.ext.toggle.label', { ext: ext.ext })"
              :data-testid="`file-types-ext-${ext.ext}`"
              @update:model-value="(v) => toggleExt(ext, v === true)"
            />
            <span class="font-data">.{{ ext.ext }}</span>
            <span
              class="ml-auto text-muted-foreground"
              :data-testid="`file-types-handler-${ext.ext}`"
              >{{
                handlerText(ext)
              }}</span
            >
          </div>
        </Disclosure>
      </Field>
    </template>
  </div>
</template>
