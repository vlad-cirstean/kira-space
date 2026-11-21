<script setup lang="ts">
import type { ApiVariable } from '@shared/domain/variables';
import CodiconIcon from '@theme/CodiconIcon.vue';
import MaskedValue from '@theme/components/MaskedValue.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Button } from '@theme/components/ui/button';
import { TableCell } from '@theme/components/ui/table';
import ValueRevealButton from '@theme/components/ValueRevealButton.vue';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';
import { cn } from '@theme/lib/utils';
import { computed, ref, watch } from 'vue';
import InlineValueInput from './InlineValueInput.vue';
import SecretCellInput from './SecretCellInput.vue';
import { cellKey, useVariablesMatrixStore } from './state/variablesMatrix';

// One cell of the Variables tab: this file's value for this name. A plain value, a secret (masked,
// set or not), the collection default shown dimmed, or nothing. Editing is inline: Enter or blur
// commits, Escape cancels. The parent turns the emitted intent into an edit call.
const props = defineProps<{
  ownerId: string;
  /** The owner's own name, for tooltips. */
  ownerName: string;
  name: string;
  /** This file's row, if it has one. */
  variable?: ApiVariable;
  /** The collection default's row, shown dimmed in an environment cell with no row of its own. */
  inherited?: ApiVariable;
  /** The default column's cell: it inherits nothing. */
  isDefault: boolean;
  collectionName: string;
  /** Another file holds this name as a secret, so a value typed here is one too. */
  rowSecret: boolean;
  /** The file has a problem: its cells are read-only until it is fixed. */
  readonly: boolean;
  /** Shared with agents. */
  shared: boolean;
  /** Every environment overrides this default. */
  overridden: boolean;
  /** The revealed plaintext of a secret; undefined until revealed. */
  revealed?: string;
}>();

const emit = defineEmits<{
  save: [value: string];
  saveSecret: [value: string];
  clear: [];
  reveal: [];
}>();

const matrix = useVariablesMatrixStore();
const key = computed(() => cellKey(props.ownerId, props.name));
const editing = computed(() => matrix.editing === key.value);

const isSecret = computed(() => props.variable?.isSecret ?? props.rowSecret);
const kind = computed<'value' | 'secret' | 'secretEmpty' | 'inherited' | 'missing' | 'empty'>(
  () => {
    if (props.variable) return props.variable.isSecret ? 'secret' : 'value';
    if (props.rowSecret) return 'secretEmpty';
    if (!props.isDefault && props.inherited) return 'inherited';
    return props.isDefault ? 'empty' : 'missing';
  },
);
const secretSet = computed(() => kind.value === 'secret' && props.variable?.secretSet === true);
const plain = computed(() => (kind.value === 'value' ? (props.variable?.value ?? '') : ''));

function begin(): void {
  if (props.readonly) return;
  matrix.startEditing(key.value);
}

function commit(value: string): void {
  matrix.stopEditing();
  if (isSecret.value) {
    emit('saveSecret', value);
    return;
  }
  if (value === '') {
    if (props.variable) emit('clear');
    return;
  }
  emit('save', value);
}

function cancel(): void {
  matrix.stopEditing();
}

const visible = ref(false);
watch(
  () => props.revealed,
  (next, previous) => {
    if (previous === undefined && next !== undefined) visible.value = true;
  },
);
function onEye(): void {
  if (props.revealed === undefined) emit('reveal');
  else visible.value = !visible.value;
}

const inheritedTooltip = computed(() =>
  t('studio.api.variables.cell.inherited.tooltip', {
    collection: props.collectionName,
    environment: props.ownerName,
  }),
);
const surface = computed(() =>
  cn('group/cell focus-visible:focus-ring', props.readonly ? 'cursor-default' : 'cursor-text'),
);
</script>

<template>
  <TableCell
    tabindex="0"
    :class="surface"
    :data-testid="`matrix-cell-${name}-${ownerId}`"
    :data-kind="kind"
    :data-shared="shared || undefined"
    @click="kind === 'secret' ? undefined : begin()"
  >
    <div class="flex min-w-0 items-center gap-1">
      <SecretCellInput
        v-if="editing && isSecret"
        :set="secretSet"
        :label="t('studio.api.variables.cell.input.aria', { name, owner: ownerName })"
        :placeholder="t('studio.api.variables.cell.secretInput.placeholder')"
        @commit="commit"
        @cancel="cancel"
      />
      <InlineValueInput
        v-else-if="editing"
        :value="plain"
        :label="t('studio.api.variables.cell.input.aria', { name, owner: ownerName })"
        allow-empty
        data-testid="matrix-cell-input"
        @commit="commit"
        @cancel="cancel"
      />
      <template v-else-if="kind === 'value'">
        <TextTooltip v-if="overridden" :text="t('studio.api.variables.cell.overridden.tooltip')"
          ><span class="min-w-0 truncate font-data text-kira-md text-subtle">{{
            plain
          }}</span></TextTooltip
        >
        <span v-else class="min-w-0 font-data text-kira-md"><TruncatedText :text="plain" /></span>
      </template>
      <template v-else-if="kind === 'secret'">
        <template v-if="secretSet">
          <span
            v-if="visible && revealed !== undefined"
            class="min-w-0 flex-1 font-data text-kira-md"
            ><TruncatedText :text="revealed" /></span
          >
          <MaskedValue v-else class="flex-1" />
          <ValueRevealButton
            :shown="visible && revealed !== undefined"
            :data-testid="`matrix-reveal-${name}-${ownerId}`"
            @click.stop="onEye"
          />
          <TooltipIconButton
            v-if="!readonly"
            :icon="ICON.edit"
            :label="t('studio.api.variables.cell.replace.label')"
            :data-testid="`matrix-replace-${name}-${ownerId}`"
            @click.stop="begin"
          />
        </template>
        <Button
          v-else
          variant="toolbar"
          size="kira"
          data-tone="warn"
          class="-ml-2"
          :disabled="readonly"
          :data-testid="`matrix-secret-set-${name}-${ownerId}`"
          @click.stop="begin"
          ><CodiconIcon :name="ICON.warning" :size="13" />
          {{ t('common.configSecrets.set.label') }}</Button
        >
      </template>
      <Button
        v-else-if="kind === 'secretEmpty'"
        variant="toolbar"
        size="kira"
        data-tone="warn"
        class="-ml-2"
        :disabled="readonly"
        :data-testid="`matrix-secret-set-${name}-${ownerId}`"
        @click.stop="begin"
        ><CodiconIcon :name="ICON.warning" :size="13" />
        {{ t('common.configSecrets.set.label') }}</Button
      >
      <TextTooltip v-else-if="kind === 'inherited'" :text="inheritedTooltip">
        <MaskedValue v-if="inherited?.isSecret" />
        <span v-else class="min-w-0 truncate font-data text-kira-md italic text-subtle">{{
          inherited?.value
        }}</span>
      </TextTooltip>
      <span v-else-if="kind === 'missing'" class="flex items-center gap-1 text-kira-sm text-warn"
        ><CodiconIcon :name="ICON.warning" :size="12" />
        {{ t('common.unset.status') }}</span
      >
      <span
        v-else
        class="text-kira-sm text-subtle opacity-0 group-hover/cell:opacity-100 group-focus-visible/cell:opacity-100"
        >{{
          t('studio.api.variables.cell.setDefault.placeholder')
        }}</span
      >
      <TextTooltip v-if="shared && !editing" :text="t('studio.api.variables.cell.shared.tooltip')"
        ><CodiconIcon name="hubot" :size="12" class="ml-auto shrink-0 text-subtle" /></TextTooltip
      >
    </div>
  </TableCell>
</template>
