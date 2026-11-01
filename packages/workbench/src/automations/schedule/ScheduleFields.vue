<script setup lang="ts">
import type { ScriptKind, ScriptParam, ScriptSchedule } from '@shared/domain/scripts';
import { keepPreviousData, useQuery } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import { Button } from '@theme/components/ui/button';
import { Card } from '@theme/components/ui/card';
import { Checkbox } from '@theme/components/ui/checkbox';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@theme/components/ui/command';
import { Field, FieldDescription, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Label } from '@theme/components/ui/label';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Popover, PopoverContent, PopoverTrigger } from '@theme/components/ui/popover';
import { RadioGroup, RadioGroupItem } from '@theme/components/ui/radio-group';
import { t } from '@theme/i18n';
import { refDebounced } from '@vueuse/core';
import { formatZonedWeekdayClock } from '@workbench/behaviour/format';
import { TIMING } from '@workbench/behaviour/timing';
import QueryState from '@workbench/components/QueryState.vue';
import { computed, ref, useId, watch } from 'vue';
import { errorText } from '../../bridge/codedError';
import { useAutomationsModule } from '../module';
import AdeContextFields from '../run/AdeContextFields.vue';
import ParamsForm from '../run/ParamsForm.vue';
import { useNextFires } from './scheduleQueries';
import { schedulePresets, timezones } from './scheduleText';

// The recurring-script part of the script editor: when, where and how a scheduled run starts.
// `scriptId` is null for a script not saved yet; the ADE task picker needs a saved script.
const props = defineProps<{
  scriptId: string | null;
  kind: ScriptKind;
  params: readonly ScriptParam[];
  /** Secret params that will have a saved value once the script is saved. */
  savedSecrets: readonly string[];
}>();
const schedule = defineModel<ScriptSchedule>({ required: true });
// False while the cron or zone is refused, so the editor can hold Save.
const emit = defineEmits<{ valid: [ok: boolean] }>();
const ctx = useAutomationsModule();
const idBase = useId();

function patch(change: Partial<ScriptSchedule>): void {
  schedule.value = { ...schedule.value, ...change };
}

const cronNow = computed(() => schedule.value.cron);
const cron = refDebounced(cronNow, TIMING.preview);
const fires = useNextFires(cron, () => schedule.value.timezone, 3);
const nextText = computed(() =>
  (fires.data.value ?? [])
    .map((ms) => formatZonedWeekdayClock(ms, schedule.value.timezone))
    .join(', '),
);
const cronError = computed(() => {
  if (!fires.isError.value) return null;
  const e = fires.error.value;
  return errorText(e);
});

watch(cronError, (e) => emit('valid', e === null), { immediate: true });

const presets = computed(() => schedulePresets());
const zoneOpen = ref(false);
const zones = timezones();
function pickZone(zone: string): void {
  patch({ timezone: zone });
  zoneOpen.value = false;
}

const secrets = computed(() => props.params.filter((p) => p.secret));
const plain = computed(() => props.params.filter((p) => !p.secret));
const values = computed({
  get: () =>
    Object.fromEntries(
      plain.value.map((p) => [p.name, schedule.value.params[p.name] ?? [...p.default]]),
    ),
  set: (v: Record<string, string[]>) => patch({ params: v }),
});

// "Run without asking" needs every required secret saved: a schedule never holds one itself.
const askLocked = computed(() =>
  secrets.value.some((p) => p.required && !props.savedSecrets.includes(p.name)),
);
watch(
  askLocked,
  (locked) => {
    if (locked && !schedule.value.confirm) patch({ confirm: true });
  },
  { immediate: true },
);

const where = ref<'folder' | 'task'>(schedule.value.taskId === '' ? 'folder' : 'task');
function setWhere(w: 'folder' | 'task'): void {
  where.value = w;
  if (w === 'folder') patch({ taskId: '', branchId: '' });
}

const tasks = useQuery({
  queryKey: computed(() => [
    'scriptRuns',
    'scheduleTasks',
    props.scriptId,
    schedule.value.taskId,
    schedule.value.branchId,
  ]),
  queryFn: () =>
    ctx.runs.preview({
      scriptId: props.scriptId ?? '',
      params: {},
      prompt: null,
      taskId: schedule.value.taskId,
      branchId: schedule.value.branchId,
      listTasks: true,
    }),
  enabled: computed(() => ctx.ade && where.value === 'task' && props.scriptId !== null),
  placeholderData: keepPreviousData,
  staleTime: 0,
});
const taskId = computed({
  get: () => schedule.value.taskId,
  set: (v: string) => patch({ taskId: v }),
});
const branchId = computed({
  get: () => schedule.value.branchId,
  set: (v: string) => patch({ branchId: v }),
});
const appName = computed(() =>
  ctx.ade ? t('common.product.kiraSpace.label') : t('common.product.kiraStudio.label'),
);
</script>

<template>
  <Card class="gap-3" data-testid="schedule-fields">
    <Field>
      <FieldLabel :for="`${idBase}-cron`">{{ t('automations.schedule.repeat.label') }}</FieldLabel>
      <div class="flex items-center gap-1.5">
        <Input
          :id="`${idBase}-cron`"
          :model-value="schedule.cron"
          spellcheck="false"
          class="font-data"
          :placeholder="t('automations.schedule.cronExample.example')"
          data-testid="schedule-cron"
          @update:model-value="(v) => patch({ cron: String(v) })"
        />
        <NativeSelect
          model-value=""
          variant="bordered"
          size="kira-lg"
          :aria-label="t('automations.schedule.presets.aria')"
          data-testid="schedule-preset"
          @update:model-value="(v) => v !== '' && patch({ cron: String(v) })"
        >
          <option value="">{{ t('automations.schedule.presets.option') }}</option>
          <option v-for="p in presets" :key="p.cron" :value="p.cron">{{ p.label }}</option>
        </NativeSelect>
      </div>
      <FieldError v-if="cronError" data-testid="schedule-cron-error">{{ cronError }}</FieldError>
      <FieldDescription v-else-if="nextText" data-testid="schedule-next">{{
        t('automations.schedule.next.label', { times: nextText })
      }}</FieldDescription>
      <FieldDescription v-else>{{ t('automations.schedule.cronHint.hint') }}</FieldDescription>
    </Field>

    <Field>
      <FieldLabel :for="`${idBase}-zone`">{{
        t('automations.schedule.timezone.label')
      }}</FieldLabel>
      <Popover v-model:open="zoneOpen">
        <PopoverTrigger as-child>
          <Button
            :id="`${idBase}-zone`"
            variant="dialog"
            size="kira-lg"
            class="justify-between"
            data-testid="schedule-timezone"
          >
            <span class="truncate">{{ schedule.timezone }}</span>
            <CodiconIcon name="chevron-down" :size="12" />
          </Button>
        </PopoverTrigger>
        <PopoverContent class="w-96 p-0">
          <Command
            :model-value="schedule.timezone"
            @update:model-value="(v) => pickZone(String(v))"
          >
            <CommandInput :placeholder="t('automations.schedule.findTimezone.placeholder')" />
            <CommandList>
              <CommandEmpty>{{ t('automations.schedule.noTimezone.empty') }}</CommandEmpty>
              <CommandGroup>
                <CommandItem
                  v-for="z in zones"
                  :key="z"
                  :value="z"
                  :data-testid="`schedule-zone-${z}`"
                  >{{
                    z
                  }}</CommandItem
                >
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </Field>

    <Field orientation="horizontal">
      <Checkbox
        :id="`${idBase}-enabled`"
        :model-value="schedule.enabled"
        data-testid="schedule-enabled"
        @update:model-value="(v) => patch({ enabled: Boolean(v) })"
      />
      <Label :for="`${idBase}-enabled`">{{ t('automations.schedule.enabled.label') }}</Label>
    </Field>

    <Field>
      <div class="flex items-center gap-2">
        <Checkbox
          :id="`${idBase}-quiet`"
          :model-value="!schedule.confirm"
          :disabled="askLocked"
          data-testid="schedule-quiet"
          @update:model-value="(v) => patch({ confirm: !v })"
        />
        <Label :for="`${idBase}-quiet`">{{ t('automations.schedule.quiet.label') }}</Label>
      </div>
      <FieldDescription v-if="askLocked" data-testid="schedule-quiet-why">
        {{ t('automations.schedule.askLocked.hint') }}
      </FieldDescription>
      <FieldDescription v-else-if="schedule.confirm">{{
        t('automations.schedule.askEach.hint')
      }}</FieldDescription>
      <FieldDescription v-else>{{ t('automations.schedule.quietWhy.hint') }}</FieldDescription>
    </Field>

    <Field v-if="kind === 'script'">
      <FieldLabel :for="`${idBase}-timeout`">{{ t('common.field.timeout.label') }}</FieldLabel>
      <Input
        :id="`${idBase}-timeout`"
        :model-value="schedule.timeout"
        :placeholder="t('automations.schedule.timeoutExample.example')"
        class="w-32 font-data"
        data-testid="schedule-timeout"
        @update:model-value="(v) => patch({ timeout: String(v) })"
      />
      <FieldDescription>{{ t('automations.schedule.timeoutRange.hint') }}</FieldDescription>
    </Field>

    <ParamsForm v-if="plain.length > 0" v-model="values" :params="plain" />

    <Field v-if="ctx.ade">
      <FieldLabel>{{ t('automations.schedule.where.label') }}</FieldLabel>
      <RadioGroup
        :model-value="where"
        class="gap-1.5"
        data-testid="schedule-where"
        @update:model-value="(v) => setWhere(v === 'task' ? 'task' : 'folder')"
      >
        <div class="flex items-center gap-2">
          <RadioGroupItem
            :id="`${idBase}-folder`"
            value="folder"
            data-testid="schedule-where-folder"
          />
          <Label :for="`${idBase}-folder`">{{ t('automations.schedule.whereFolder.label') }}</Label>
        </div>
        <div class="flex items-center gap-2">
          <RadioGroupItem :id="`${idBase}-task`" value="task" data-testid="schedule-where-task" />
          <Label :for="`${idBase}-task`">{{ t('automations.schedule.whereTask.label') }}</Label>
        </div>
      </RadioGroup>
      <template v-if="where === 'task'">
        <FieldDescription v-if="scriptId === null" data-testid="schedule-task-save">
          {{ t('automations.schedule.saveFirst.hint') }}
        </FieldDescription>
        <QueryState v-else :query="tasks" variant="inline" :is-empty="() => false">
          <template #loading />
          <template #error="{ message }">
            <FieldError data-testid="schedule-task-error">{{ message }}</FieldError>
          </template>
          <template #default="{ data }">
            <AdeContextFields
              v-model:task-id="taskId"
              v-model:branch-id="branchId"
              :needs="data.needs"
              :ade="data.ade"
            />
          </template>
        </QueryState>
      </template>
    </Field>

    <FieldDescription data-testid="schedule-note">
      {{ t('automations.schedule.note.hint', { app: appName }) }}
    </FieldDescription>
  </Card>
</template>
