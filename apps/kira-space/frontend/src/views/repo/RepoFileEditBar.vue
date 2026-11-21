<script setup lang="ts">
import CodiconIcon from '@theme/CodiconIcon.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { t } from '@theme/i18n';
import { ICON } from '@theme/icons';

defineProps<{ editing: boolean; dirty: boolean; saving: boolean; locked: boolean }>();
defineEmits<{ edit: []; save: []; done: [] }>();
</script>

<template>
  <div class="ml-auto flex items-center gap-1.5">
    <Button
      v-if="!editing"
      variant="toolbar"
      size="kira"
      :disabled="locked"
      data-testid="repo-file-edit"
      @click="$emit('edit')"
      ><CodiconIcon :name="ICON.edit" :size="13" />
      {{ t('common.action.edit.label') }}
    </Button>
    <template v-else>
      <Badge v-if="dirty" variant="warn" data-testid="repo-file-modified">
        {{ t('common.state.unsavedChanges.status') }}
      </Badge>
      <TextTooltip :text="t('common.action.save.label')" shortcut="form.save">
        <Button
          variant="toolbar"
          size="kira"
          :disabled="!dirty"
          :pending="saving"
          data-testid="repo-file-save"
          @click="$emit('save')"
        >
          <CodiconIcon name="save" :size="13" />
          {{ t('common.action.save.label') }}
        </Button>
      </TextTooltip>
      <Button
        variant="toolbar"
        size="kira"
        :disabled="locked"
        data-testid="repo-file-done"
        @click="$emit('done')"
        ><CodiconIcon name="check" :size="13" />
        {{ t('common.done.status') }}
      </Button>
    </template>
  </div>
</template>
