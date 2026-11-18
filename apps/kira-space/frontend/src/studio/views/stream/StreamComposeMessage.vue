<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import { Button } from '@theme/components/ui/button';
import { Field, FieldError, FieldLabel } from '@theme/components/ui/field';
import { Input } from '@theme/components/ui/input';
import { Textarea } from '@theme/components/ui/textarea';
import { t } from '@theme/i18n';
import { wrapSelectionOnType } from '@theme/wrapSelection';
import { useAsyncAction } from '@workbench/behaviour/asyncAction';
import PanelHeader from '@workbench/components/PanelHeader.vue';
import { computed, ref, useId } from 'vue';
import { produceKafkaMessage, sendSqsMessage } from './mutations';
import { useStreamViewStore } from './state';

// The "Add message" panel — Kafka gets key/body/headers (kafka/produce.ts's three
// sentinel fields); SQS gets body/headers but no key (SendMessage has no key concept on this
// app's model — see sqs/mutate.ts's own scoping note; SQS has MessageAttributes support, so
// headers is now shared by both kinds). One popover, one shape switch, rather than
// two near-identical components.
const props = defineProps<{
  tabId: string;
  kind: 'kafka' | 'sqs';
}>();
const emit = defineEmits<{ close: [] }>();

const HEADERS_EXAMPLE = '{"source": "manual"}';
const key = ref('');
const body = ref('');
const headers = ref('');
const keyId = useId();
const bodyId = useId();
const headersId = useId();

const isKafka = computed(() => props.kind === 'kafka');
const streamViewStore = useStreamViewStore();
const canSubmit = computed(
  () => body.value.trim() !== '' && !send.pending.value && !streamViewStore.isStale(props.tabId),
);

const send = useAsyncAction(() => {
  const headersValue = headers.value.trim() === '' ? null : headers.value;
  return isKafka.value
    ? produceKafkaMessage(props.tabId, {
        key: key.value.trim() === '' ? null : key.value,
        body: body.value,
        headers: headersValue,
      })
    : sendSqsMessage(props.tabId, body.value, headersValue);
});
const error = send.error;

async function submit(): Promise<void> {
  if (!canSubmit.value) return;
  if (await send.run()) emit('close');
}
</script>

<template>
  <div class="flex flex-col">
    <PanelHeader>
      <template #start><CodiconIcon name="add" :size="13" /></template>
      {{ t(isKafka ? 'studio.stream.produce.label' : 'studio.stream.send.label') }}
      <template #actions>
        <TooltipIconButton
          icon="close"
          :label="t('common.action.close.label')"
          @click="emit('close')"
        />
      </template>
    </PanelHeader>

    <div class="flex flex-col gap-1.5 p-2">
      <Field v-if="isKafka">
        <FieldLabel :for="keyId">{{ t('studio.stream.compose.key.label') }}</FieldLabel>
        <Input
          :id="keyId"
          :model-value="key"
          :placeholder="t('common.state.none.option')"
          class="font-data"
          data-testid="stream-add-message-key"
          @update:model-value="(v) => (key = String(v))"
        />
      </Field>

      <Field>
        <FieldLabel :for="bodyId">{{ t('common.body.label') }}</FieldLabel>
        <Textarea
          class="min-h-32 max-h-90 resize-y font-data"
          :id="bodyId"
          v-model="body"
          :placeholder="t('studio.stream.compose.body.placeholder')"
          data-testid="stream-add-message-body"
          @keydown="wrapSelectionOnType"
        />
      </Field>

      <Field>
        <FieldLabel :for="headersId">{{ t('studio.stream.compose.headers.label') }}</FieldLabel>
        <Textarea
          class="font-data"
          :id="headersId"
          v-model="headers"
          :placeholder="HEADERS_EXAMPLE"
          data-testid="stream-add-message-headers"
          @keydown="wrapSelectionOnType"
        />
      </Field>

      <FieldError v-if="error" data-testid="stream-add-message-error">{{ error }}</FieldError>
    </div>

    <div class="flex justify-end gap-1.5 py-1.5 px-2">
      <Button variant="dialog" size="kira-lg" @click="emit('close')">{{
        t('common.action.cancel.label')
      }}</Button>
      <Button
        variant="dialog-primary"
        size="kira-lg"
        :disabled="!canSubmit"
        data-testid="stream-add-message-submit"
        @click="submit"
      >
        {{ t(isKafka ? 'studio.stream.produceAction.label' : 'common.action.send.label') }}
      </Button>
    </div>
  </div>
</template>
