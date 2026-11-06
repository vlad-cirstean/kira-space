<script setup lang="ts">
import type { HttpFormDataFieldState } from '@shared/domain/http';
import { contentTypeForFilename } from '@shared/domain/objectStore';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Input } from '@theme/components/ui/input';
import { InputGroup, InputGroupTextarea } from '@theme/components/ui/input-group';
import { NativeSelect } from '@theme/components/ui/native-select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { t } from '@theme/i18n';
import { formatBytes } from '@workbench/behaviour/format';
import AutocompleteField from '@workbench/components/AutocompleteField.vue';
import { templateToken } from '@workbench/editor/fieldCompletion';
import type { VariableSupport } from '../../api/state/variableCompletion';
import { patchHttpRequestTabState } from '../../api/tabs';
import type { HttpRequestTabRecord } from '../../state/tabDomain';
import FieldRowsTable from '../shared/fields/FieldRowsTable.vue';
import { chooseBodyFile } from './files';

// P3 C8/D4/D15: form-data over C6's shared table, with real file fields. D4's whole point: a
// picked file's bytes never reach here — chooseBodyFile returns only {path, name, size}, and only
// `path` (D8's own `path`/`fileName`/`fileSize` fields) ever lands in state or on the wire.
const props = defineProps<{
  tab: HttpRequestTabRecord;
  /** P15b D4: HttpRequestView.vue's own variableSupport(...) — this table overrides FieldRowsTable's
   *  own `value` slot (a text/file kind switch), so it wires the AutocompleteField itself rather
   *  than through `valueVariableSupport`. */
  variables?: VariableSupport;
  /** P16 D13: HttpRequestView.vue's own #toolbar-2 filter box, forwarded to FieldRowsTable. */
  filterQuery?: string;
  /** P22b D7: HttpRequestView.vue's own persisted description-column toggle, forwarded to
   *  FieldRowsTable. */
  showDescriptions?: boolean;
}>();

function blankField(): HttpFormDataFieldState {
  return {
    name: '',
    kind: 'text',
    value: '',
    path: '',
    fileName: '',
    fileSize: 0,
    contentType: '',
    enabled: true,
    description: '',
  };
}

function onUpdateRows(formData: HttpFormDataFieldState[]): void {
  patchHttpRequestTabState(props.tab.id, { formData });
}

function updateRow(index: number, patch: Partial<HttpFormDataFieldState>): void {
  const rows = props.tab.state.formData;
  const next = [...rows];
  if (index === next.length) next.push(blankField());
  next[index] = { ...next[index], ...patch };
  patchHttpRequestTabState(props.tab.id, { formData: next });
}

function onKindChange(index: number, value: unknown): void {
  updateRow(index, { kind: String(value) as 'text' | 'file' });
}

// D4: chooseBodyFile is Go's own picker, wrapped once (files.ts) — the result never carries the
// file's bytes, only {path, name, size}. contentTypeForFilename prefills the per-part override
// from the same small extension table UploadObjectDialog.vue already uses (object-store.ts).
async function onChooseFile(index: number): Promise<void> {
  const file = await chooseBodyFile(t('studio.http.body.chooseFile.label'));
  if (!file) return;
  updateRow(index, {
    path: file.path,
    fileName: file.name,
    fileSize: file.size,
    contentType: contentTypeForFilename(file.name),
  });
}

function onClearFile(index: number): void {
  updateRow(index, { path: '', fileName: '', fileSize: 0 });
}
</script>

<template>
  <FieldRowsTable
    :rows="tab.state.formData"
    :blank-row="blankField"
    show-enabled
    :name-placeholder="t('common.field.key.label')"
    :value-placeholder="t('common.field.value.label')"
    testid-prefix="http-formdata"
    container-testid="http-formdata-table"
    :filter-query="filterQuery"
    :show-descriptions="showDescriptions"
    @update:rows="onUpdateRows"
  >
    <template #value="{ row, update }">
      <div v-if="row.kind === 'text'" class="flex-1 min-w-0">
        <AutocompleteField
          v-if="variables"
          grow
          class="w-full"
          :model-value="row.value"
          :placeholder="t('common.field.value.label')"
          data-testid="http-formdata-value"
          :candidates="variables.candidates"
          :token-at="templateToken"
          :range-highlights="variables.rangeHighlights"
          :hover-at="variables.hoverAt"
          @update:model-value="update"
        />
        <InputGroup v-else>
          <InputGroupTextarea
            :model-value="row.value"
            :placeholder="t('common.field.value.label')"
            data-testid="http-formdata-value"
            rows="1"
            @update:model-value="update(String($event))"
          />
        </InputGroup>
      </div>
    </template>

    <template #trailing="{ row, index }">
      <NativeSelect
        variant="bordered"
        data-testid="http-formdata-kind"
        :model-value="row.kind"
        @update:model-value="(v) => onKindChange(index, v)"
      >
        <option value="text">{{ t('common.field.text.label') }}</option>
        <option value="file">{{ t('common.field.file.label') }}</option>
      </NativeSelect>
      <template v-if="row.kind === 'file'">
        <Button
          variant="toolbar"
          size="kira"
          data-testid="http-formdata-choose-file"
          @click="onChooseFile(index)"
        >
          {{ t('studio.http.body.chooseFile.label') }}
        </Button>
        <Tooltip v-if="row.fileName">
          <TooltipTrigger as-child>
            <span
              class="text-kira-sm text-muted-foreground whitespace-nowrap p-0"
              data-testid="http-formdata-file-caption"
            >
              {{ row.fileName }}
              ({{ formatBytes(row.fileSize) }})
            </span>
          </TooltipTrigger>
          <TooltipContent>{{ row.path }}</TooltipContent>
        </Tooltip>
        <TooltipIconButton
          v-if="row.fileName"
          icon="close"
          :label="t('studio.http.body.clearFile.label')"
          data-testid="http-formdata-clear-file"
          @click="onClearFile(index)"
        />
        <div class="flex-1 min-w-0">
          <Input
            :model-value="row.contentType"
            :placeholder="t('studio.http.body.contentType.placeholder')"
            data-testid="http-formdata-content-type"
            @update:model-value="(v) => updateRow(index, { contentType: String(v) })"
          />
        </div>
      </template>
    </template>
  </FieldRowsTable>
</template>
