<script setup lang="ts">
import type { GrpcSchemaWire } from '@shared/domain/grpc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import EmptyState from '@theme/components/EmptyState.vue';
import InlineState from '@theme/components/InlineState.vue';
import SearchField from '@theme/components/SearchField.vue';
import SecondaryTabs from '@theme/components/SecondaryTabs.vue';
import SectionHeading from '@theme/components/SectionHeading.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { rowVariants } from '@theme/lib/rowVariants';
import { cn } from '@theme/lib/utils';
import { matchesQuery } from '@workbench/behaviour/match';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, ref } from 'vue';
import type { GrpcRequestTabRecord } from '../../../state/tabDomain';
import { patchGrpcRequestTabState } from '../../api/tabs';
import { control } from '../../bridge/control';
import { useGrpcRequestViewStore } from './state';

// D13's Schema pane: the source selector (Reflection / .proto file + import paths + Reload) above
// a service→method list — inside the tab, not the left panel (D13's own reasoning: a schema is a
// property of one request's target, not of the workspace).
const props = defineProps<{
  tab: GrpcRequestTabRecord;
  schema: GrpcSchemaWire | null;
  loading: boolean;
  error: string | null;
}>();
const emit = defineEmits<{ reload: [] }>();

const SOURCE_OPTIONS = computed(() => [
  {
    value: 'reflection' as const,
    label: t('studio.grpc.schema.source.reflection.option'),
    testid: 'grpc-source-reflection',
  },
  {
    value: 'proto' as const,
    label: t('studio.grpc.schema.source.proto.option'),
    testid: 'grpc-source-proto',
  },
]);

function setDescriptorMode(mode: 'reflection' | 'proto'): void {
  patchGrpcRequestTabState(props.tab.id, { descriptorMode: mode });
}

function onTargetInput(value: string): void {
  patchGrpcRequestTabState(props.tab.id, { target: value });
}

async function chooseProtoFile(): Promise<void> {
  const chosen = await control.filesChooseOpen({
    title: t('studio.grpc.schema.chooseProto.title'),
    filters: [{ name: t('studio.grpc.schema.protoFilter.label'), extensions: ['proto'] }],
  });
  if (chosen.canceled || !chosen.file) return;
  patchGrpcRequestTabState(props.tab.id, { protoPath: chosen.file.path });
}

// D4: import paths default to the .proto file's own directory (Go's own resolveProto), and the
// user can add more — FilesService.ChooseOpen has no directory-picking mode (P3 D4's own file-only
// scope), so an extra path is typed and confirmed with Enter rather than browsed to.
const newImportPath = ref('');
function addImportPath(): void {
  const path = newImportPath.value.trim();
  if (!path) return;
  patchGrpcRequestTabState(props.tab.id, {
    importPaths: [...props.tab.state.importPaths, path],
  });
  newImportPath.value = '';
}

function removeImportPath(i: number): void {
  patchGrpcRequestTabState(props.tab.id, {
    importPaths: props.tab.state.importPaths.filter((_, idx) => idx !== i),
  });
}

// P16 D15: matches service name or method name — a service whose own name matches shows all its
// methods; otherwise only the methods that themselves match, and a service with no match at all
// (name or any method) drops out entirely.
const grpcRequestViewStore = useGrpcRequestViewStore();
const filterQuery = computed({
  get: () => grpcRequestViewStore.runtime[props.tab.id]?.schemaFilter ?? '',
  set: (v: string) => grpcRequestViewStore.setSchemaFilter(props.tab.id, v),
});
const isFiltered = computed(() => filterQuery.value.trim() !== '');
const filteredServices = computed(() => {
  const services = props.schema?.services ?? [];
  if (!isFiltered.value) return services;
  const q = filterQuery.value;
  return services.flatMap((svc) => {
    const svcMatches = matchesQuery(svc.name, q);
    const methods = svcMatches ? svc.methods : svc.methods.filter((m) => matchesQuery(m.name, q));
    return methods.length > 0 ? [{ ...svc, methods }] : [];
  });
});

function selectMethod(service: string, method: string): void {
  const m = props.schema?.services
    .find((s) => s.fullName === service)
    ?.methods.find((mm) => mm.name === method);
  patchGrpcRequestTabState(props.tab.id, {
    service,
    method,
    message: m?.requestTemplate ?? props.tab.state.message,
    requestPane: 'message',
  });
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-auto" data-testid="grpc-schema-browser">
    <ViewToolbar>
      <SecondaryTabs
        variant="segmented"
        :model-value="tab.state.descriptorMode"
        :items="SOURCE_OPTIONS"
        data-testid="grpc-source-toggle"
        @update:model-value="(v) => setDescriptorMode(v as 'reflection' | 'proto')"
      />
      <template v-if="tab.state.descriptorMode === 'reflection'">
        <Input
          size="kira"
          :model-value="tab.state.target"
          :placeholder="t('studio.grpc.target.example')"
          class="flex-1"
          data-testid="grpc-schema-target"
          @update:model-value="onTargetInput($event.toString())"
        />
      </template>
      <template v-else>
        <Input
          size="kira"
          :model-value="tab.state.protoPath"
          :placeholder="t('studio.grpc.schema.noProto.placeholder')"
          readonly
          class="flex-1"
          data-testid="grpc-proto-path"
        />
        <Button
          variant="toolbar"
          size="kira"
          data-testid="grpc-choose-proto"
          @click="chooseProtoFile"
        >
          {{ t('common.action.choose.label') }}
        </Button>
      </template>
      <Button
        variant="toolbar"
        size="kira"
        data-testid="grpc-schema-reload"
        :pending="loading"
        @click="emit('reload')"
      >
        <CodiconIcon :name="ICON.refresh" :size="13" />
        {{ t('common.action.refresh.label') }}
      </Button>
    </ViewToolbar>

    <div
      v-if="tab.state.descriptorMode === 'proto'"
      class="flex flex-col gap-0.5 px-1.5 py-1"
      data-testid="grpc-import-paths"
    >
      <SectionHeading :label="t('studio.grpc.schema.importPaths.label')" />
      <!-- D14: a real cap + scroll, so a large .proto tree's import list can never push the
           source row and the whole service browser off the pane. -->
      <div class="flex max-h-32 flex-col overflow-auto">
        <div v-for="(p, i) in tab.state.importPaths" :key="i" :class="rowVariants()">
          <span class="min-w-0 text-kira-sm font-data"><TruncatedText :text="p" /></span>
          <TooltipIconButton
            :icon="ICON.close"
            :label="t('common.action.remove.label')"
            class="ml-auto"
            @click="removeImportPath(i)"
          />
        </div>
        <InlineState v-if="tab.state.importPaths.length === 0" kind="empty">
          {{ t('studio.grpc.schema.noImportPaths.empty') }}
        </InlineState>
      </div>
      <div class="flex items-center gap-1">
        <Input
          v-model="newImportPath"
          size="kira"
          :placeholder="t('studio.grpc.schema.addImportPath.placeholder')"
          data-testid="grpc-new-import-path"
          @keydown.enter="addImportPath"
        />
        <Button
          variant="toolbar"
          size="kira"
          data-testid="grpc-add-import-path"
          @click="addImportPath"
        >
          {{ t('common.action.add.label') }}
        </Button>
      </div>
    </div>

    <Alert strip v-if="error" variant="error" data-testid="grpc-schema-error">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <SearchField
      v-if="schema && schema.services.length > 0"
      v-model="filterQuery"
      :placeholder="t('studio.grpc.schema.filter.placeholder')"
      data-testid="grpc-schema-filter"
    />
    <div class="flex-1 min-h-0 overflow-auto px-1.5 py-1" data-testid="grpc-service-list">
      <template v-if="schema && filteredServices.length > 0">
        <div v-for="svc in filteredServices" :key="svc.fullName" class="mb-1.5">
          <SectionHeading :label="svc.name" data-testid="grpc-service-name" />
          <!-- The row's own template class list (P110 B29) supplies height/display/align-items/
               gap/padding/border-radius/color/font-size/cursor and its own hover/selected
               background; the extra classes here only supply what that list does not. -->
          <button
            v-for="m in svc.methods"
            :key="m.name"
            type="button"
            :class="
              cn(
                rowVariants({
                  selected: tab.state.service === svc.fullName && tab.state.method === m.name,
                }),
                'w-full justify-between border-0 bg-none text-left',
              )
            "
            data-testid="grpc-method-row"
            @click="selectMethod(svc.fullName, m.name)"
          >
            <span class="text-kira-md font-data">{{ m.name }}</span>
            <Badge
              v-if="m.serverStreaming || m.clientStreaming"
              variant="ok"
              data-testid="grpc-method-streaming-badge"
            >
              {{ t('shared.grpc.streaming.stream.label') }}
            </Badge>
            <Badge v-else variant="info" data-testid="grpc-method-streaming-badge">{{
              t('shared.grpc.streaming.unary.label')
            }}</Badge>
          </button>
        </div>
      </template>
      <EmptyState
        v-else-if="isFiltered && schema && schema.services.length > 0"
        data-testid="grpc-schema-filter-empty"
        :icon="ICON.search"
        :title="t('common.state.noMatches.empty')"
      />
      <EmptyState
        v-else-if="!loading"
        icon="symbol-interface"
        :title="t('studio.grpc.schema.chooseSource.empty')"
      />
    </div>
  </div>
</template>
