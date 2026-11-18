<script setup lang="ts">
import CodeBlock from '@theme/components/CodeBlock.vue';
import { Alert, AlertDescription } from '@theme/components/ui/alert';
import { Button } from '@theme/components/ui/button';
import { FieldDescription, FieldLegend, FieldSet } from '@theme/components/ui/field';
import { t } from '@theme/i18n';
import { computed } from 'vue';
import { useCopy } from '../../behaviour/clipboard';
import QueryState from '../../components/QueryState.vue';
import { useMemoryCheckHook } from '../queries';

// P273, P296: the commands that install the memory commit check in another repository.
const check = useMemoryCheckHook();
const install = computed(() => check.data.value?.installCommand ?? '');
const hook = computed(() => check.data.value?.hookLine ?? '');
const installCopy = useCopy();
const hookCopy = useCopy();
</script>

<template>
  <FieldSet data-testid="memory-check-section">
    <FieldLegend>{{ t('memory.checkHook.title') }}</FieldLegend>
    <FieldDescription>{{ t('memory.checkHook.description') }}</FieldDescription>
    <QueryState :query="check" variant="inline" :is-empty="() => false">
      <template #error="{ message }">
        <Alert variant="error">
          <AlertDescription>{{ message }}</AlertDescription>
        </Alert>
      </template>
      <template v-if="install">
        <FieldDescription>{{ t('memory.checkHook.install.hint') }}</FieldDescription>
        <CodeBlock class="select-all" data-testid="memory-check-install">{{ install }}</CodeBlock>
        <div class="flex items-center gap-1">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="memory-check-copy-install"
            @click="installCopy.copy(install)"
          >
            {{
              installCopy.copied.value
                ? t('common.copy.copied.announce')
                : t('common.copyCommand.label')
            }}
          </Button>
        </div>
        <FieldDescription>{{ t('memory.checkHook.hook.hint') }}</FieldDescription>
        <CodeBlock class="select-all" data-testid="memory-check-hook">{{ hook }}</CodeBlock>
        <div class="flex items-center gap-1">
          <Button
            variant="dialog"
            size="kira-lg"
            data-testid="memory-check-copy-hook"
            @click="hookCopy.copy(hook)"
          >
            {{
              hookCopy.copied.value
                ? t('common.copy.copied.announce')
                : t('common.copyCommand.label')
            }}
          </Button>
        </div>
      </template>
    </QueryState>
  </FieldSet>
</template>
