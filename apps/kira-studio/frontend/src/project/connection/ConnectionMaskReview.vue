<script setup lang="ts">
import type { ConnectionFileModel } from '@shared/domain/connectionConfig';
import {
  MASK_REVIEW_KINDS,
  type MaskReviewColumn,
  type MaskReviewState,
  maskRuleFor,
  reviewStateOf,
} from '@shared/domain/maskReview';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import CodiconIcon from '@theme/CodiconIcon.vue';
import SearchField from '@theme/components/SearchField.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { Checkbox } from '@theme/components/ui/checkbox';
import { FieldDescription } from '@theme/components/ui/field';
import { NativeSelect } from '@theme/components/ui/native-select';
import { t } from '@theme/i18n';
import { matchesQuery } from '@workbench/behaviour/match';
import { useVirtualRows, VIRTUAL_ROW_CLASS } from '@workbench/behaviour/virtualRows';
import QueryState from '@workbench/components/QueryState.vue';
import { useRowHeight } from '@workbench/lib/rowHeight';
import { useConfirmDialogStore } from '@workbench/state/confirmDialog';
import { computed, ref } from 'vue';
import { control } from '../../bridge/control';
import CheckboxRow from './CheckboxRow.vue';

// Column review (P534): live columns the user has not seen are withheld from the MCP agent until reviewed.
// The reviewed list is part of the file draft (`mask_review.columns`); the live list is server state.
const props = defineProps<{ connectionId: string }>();
const emit = defineEmits<{ mask: [column: MaskReviewColumn] }>();
const file = defineModel<ConnectionFileModel>({ required: true });
const confirmDialogStore = useConfirmDialogStore();
const queryClient = useQueryClient();
const { single: rowHeight } = useRowHeight();

const COLUMNS_SHOWN_MAX = 100000;
const applies = computed(() => MASK_REVIEW_KINDS.has(file.value.kind));
const on = computed(() => file.value.mask_review !== undefined);
const reviewed = computed(() => new Set(file.value.mask_review?.columns ?? []));
const rules = computed(() => file.value.masks ?? []);

const queryKey = computed(() => ['maskReviewColumns', props.connectionId]);
const columnsQuery = useQuery({
  queryKey,
  queryFn: () => control.maskReviewColumns(props.connectionId, false),
  enabled: computed(() => applies.value && !!props.connectionId),
});
const refresh = useMutation({
  mutationFn: () => control.maskReviewColumns(props.connectionId, true),
  onSuccess: (data) => queryClient.setQueryData(queryKey.value, data),
});

const live = computed(() => columnsQuery.data.value?.columns ?? []);
const loaded = computed(() => columnsQuery.data.value?.applicable === true);

interface ReviewRow {
  column: MaskReviewColumn;
  state: MaskReviewState;
  ruleKind: string | undefined;
}
const allRows = computed<ReviewRow[]>(() =>
  live.value.map((column) => ({
    column,
    state: reviewStateOf(column, rules.value, reviewed.value),
    ruleKind: maskRuleFor(column, rules.value)?.kind,
  })),
);
const needReview = computed(() => allRows.value.filter((r) => r.state === 'pending').length);

const search = ref('');
const filterChoice = ref<'all' | 'pending' | null>(null);
const filter = computed(() => filterChoice.value ?? (needReview.value > 0 ? 'pending' : 'all'));
const rows = computed(() =>
  allRows.value.filter(
    (r) =>
      (filter.value === 'all' || r.state === 'pending') && matchesQuery(r.column.ref, search.value),
  ),
);
const scroll = ref<HTMLElement | null>(null);
const { virtualItems, totalSize } = useVirtualRows({
  count: () => rows.value.length,
  rowHeight: () => rowHeight.value,
  scrollElement: scroll,
});

const status = computed(() => {
  if (!on.value) return t('studio.project.connectionDialog.privacy.reviewStatusOff.status');
  if (!loaded.value || needReview.value === 0)
    return t('studio.project.connectionDialog.privacy.reviewStatusDone.status');
  return t('studio.project.connectionDialog.privacy.reviewStatusPending.status', {
    count: needReview.value,
  });
});

function setColumns(columns: string[]): void {
  file.value.mask_review = { columns };
}

async function setOn(next: boolean): Promise<void> {
  if (next) {
    setColumns(live.value.map((c) => c.ref));
    return;
  }
  const ok = await confirmDialogStore.confirm({
    title: t('studio.project.connectionDialog.privacy.reviewOffConfirm.title'),
    message: t('studio.project.connectionDialog.privacy.reviewOffConfirm.body'),
    confirmLabel: t('studio.project.connectionDialog.privacy.reviewOffConfirm.label'),
    tone: 'danger',
  });
  if (!ok) return;
  const { mask_review: _drop, ...rest } = file.value;
  file.value = rest;
}

function toggle(ref: string, checked: boolean): void {
  const next = new Set(reviewed.value);
  if (checked) next.add(ref);
  else next.delete(ref);
  setColumns([...next]);
}

function confirmAll(): void {
  setColumns(live.value.map((c) => c.ref));
}

const STATE_LABEL = {
  masked: 'studio.project.connectionDialog.privacy.reviewMasked.status',
  reviewed: 'studio.project.connectionDialog.privacy.reviewReviewed.status',
  pending: 'studio.project.connectionDialog.privacy.reviewPending.status',
} as const;
const STATE_VARIANT = { masked: 'ok', reviewed: 'default', pending: 'warn' } as const;
</script>

<template>
  <div class="flex flex-col gap-2" data-testid="connection-privacy-review">
    <div class="text-fg font-medium">
      {{ t('studio.project.connectionDialog.privacy.reviewTitle.label') }}
    </div>
    <FieldDescription v-if="!applies" class="w-full" data-testid="connection-privacy-review-na">
      {{ t('studio.project.connectionDialog.privacy.reviewNa.body') }}
    </FieldDescription>
    <template v-else>
      <CheckboxRow
        :model-value="on"
        :disabled="!on && (!loaded || !connectionId)"
        :label="t('studio.project.connectionDialog.privacy.reviewOn.label')"
        :description="t('studio.project.connectionDialog.privacy.reviewOn.description')"
        testid="connection-privacy-review-on"
        @update:model-value="setOn"
      />
      <FieldDescription class="w-full" data-testid="connection-privacy-review-enforce">
        {{ t('studio.project.connectionDialog.privacy.reviewEnforceWho.body') }}
        {{ t('studio.project.connectionDialog.privacy.reviewEnforceHit.body') }}
        {{ t('studio.project.connectionDialog.privacy.reviewEnforceLimits.body') }}
      </FieldDescription>
      <div class="flex items-center gap-2">
        <span
          class="flex-1 text-kira-sm text-subtle"
          data-testid="connection-privacy-review-status"
          >{{
            status
          }}</span
        >
        <TooltipIconButton
          icon="refresh"
          :label="t('studio.project.connectionDialog.privacy.reviewRefresh.tooltip')"
          :disabled="!connectionId || refresh.isPending.value"
          data-testid="connection-privacy-review-refresh"
          @click="refresh.mutate()"
        />
      </div>
      <FieldDescription
        v-if="!connectionId"
        class="w-full"
        data-testid="connection-privacy-review-offline"
      >
        {{ t('studio.project.connectionDialog.privacy.reviewOffline.body') }}
      </FieldDescription>
      <QueryState
        v-else
        :query="columnsQuery"
        variant="inline"
        :is-empty="() => false"
        :loading-label="t('studio.project.connectionDialog.privacy.reviewRefresh.tooltip')"
      >
        <template #error="{ retry }">
          <div
            class="flex items-center gap-2 text-kira-sm text-subtle"
            data-testid="connection-privacy-review-offline"
          >
            <span>{{ t('studio.project.connectionDialog.privacy.reviewOffline.body') }}</span>
            <Button size="kira" variant="dialog" @click="retry">{{
              t('common.action.retry.label')
            }}</Button>
          </div>
        </template>
        <FieldDescription
          v-if="!loaded"
          class="w-full"
          data-testid="connection-privacy-review-offline"
        >
          {{ t('studio.project.connectionDialog.privacy.reviewOffline.body') }}
        </FieldDescription>
        <template v-else>
          <div class="flex items-center gap-2">
            <SearchField
              v-model="search"
              class="flex-1"
              :placeholder="t('studio.project.connectionDialog.privacy.reviewSearch.placeholder')"
              data-testid="connection-privacy-review-search"
            />
            <NativeSelect
              variant="bordered"
              :model-value="filter"
              data-testid="connection-privacy-review-filter"
              @update:model-value="(v) => (filterChoice = v === 'all' ? 'all' : 'pending')"
            >
              <option value="pending">
                {{ t('studio.project.connectionDialog.privacy.reviewPending.status') }}
              </option>
              <option value="all">
                {{ t('studio.project.connectionDialog.privacy.reviewFilterAll.option') }}
              </option>
            </NativeSelect>
            <Button
              variant="dialog"
              size="kira-lg"
              :disabled="!on || needReview === 0"
              data-testid="connection-privacy-review-confirm-all"
              @click="confirmAll"
            >
              {{ t('studio.project.connectionDialog.privacy.reviewConfirmAll.label') }}
            </Button>
          </div>
          <div
            ref="scroll"
            class="max-h-64 overflow-auto border border-border rounded-kira-sm"
            data-testid="connection-privacy-review-list"
          >
            <div v-if="rows.length === 0" class="p-2 text-kira-sm text-subtle">
              {{ t('studio.project.connectionDialog.privacy.reviewEmpty.empty') }}
            </div>
            <div v-else class="relative w-full" :style="{ height: `${totalSize}px` }">
              <div
                v-for="item in virtualItems"
                :key="rows[item.index]?.column.ref"
                :class="['flex items-center gap-2 px-2 h-row', VIRTUAL_ROW_CLASS]"
                :style="{ transform: `translateY(${item.start}px)` }"
                data-testid="connection-privacy-review-row"
                :data-ref="rows[item.index]?.column.ref"
                :data-state="rows[item.index]?.state"
              >
                <Checkbox
                  :model-value="rows[item.index]?.state !== 'pending'"
                  :disabled="!on || rows[item.index]?.state === 'masked'"
                  class="size-3.5"
                  :aria-label="t('common.markReviewed.tooltip')"
                  data-testid="connection-privacy-review-reviewed"
                  @update:model-value="
                    (v) => toggle(rows[item.index]?.column.ref ?? '', v === true)
                  "
                >
                  <CodiconIcon name="check" :size="12" />
                </Checkbox>
                <span class="flex-1 min-w-0 font-data">
                  <TruncatedText :text="rows[item.index]?.column.ref ?? ''" />
                </span>
                <span class="text-subtle text-kira-sm truncate">{{
                  rows[item.index]?.column.dataType
                }}</span>
                <Badge
                  :variant="STATE_VARIANT[rows[item.index]?.state ?? 'pending']"
                  data-testid="connection-privacy-review-state"
                >
                  {{ t(STATE_LABEL[rows[item.index]?.state ?? 'pending']) }}
                  <template v-if="rows[item.index]?.ruleKind">
                    · {{ rows[item.index]?.ruleKind }}</template
                  >
                </Badge>
                <Button
                  v-if="rows[item.index]?.state !== 'masked'"
                  size="kira"
                  variant="dialog"
                  data-testid="connection-privacy-review-mask"
                  @click="emit('mask', rows[item.index]?.column as MaskReviewColumn)"
                >
                  {{ t('studio.project.connectionDialog.privacy.reviewMask.label') }}
                </Button>
              </div>
            </div>
          </div>
          <FieldDescription
            v-if="columnsQuery.data.value?.truncated"
            class="w-full"
            data-testid="connection-privacy-review-truncated"
          >
            {{
              t('studio.project.connectionDialog.privacy.reviewTruncated.body', {
                max: String(COLUMNS_SHOWN_MAX),
              })
            }}
          </FieldDescription>
        </template>
      </QueryState>
    </template>
  </div>
</template>
