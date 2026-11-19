<script setup lang="ts">
import EmptyState from '@theme/components/EmptyState.vue';
import { t } from '@theme/i18n';
import { paneFrame, windowBody, windowTabStrip } from '@theme/lib/frames';
import { cn } from '@theme/lib/utils';
import MainView from '@workbench/components/MainView.vue';
import TabStrip from '@workbench/components/TabStrip.vue';
import TitleBarBase from '@workbench/components/TitleBar.vue';
import TitleBarTitle from '@workbench/components/TitleBarTitle.vue';
</script>

<template>
  <div class="flex h-full flex-col" data-testid="opened-files-window">
    <TitleBarBase>
      <TitleBarTitle data-testid="opened-files-title">{{
        t('space.openedFiles.window.title')
      }}</TitleBarTitle>
    </TitleBarBase>
    <div :class="windowBody">
      <section
        :class="cn(paneFrame, 'flex flex-1 flex-col')"
        :aria-label="t('space.openedFiles.window.title')"
      >
        <div :class="windowTabStrip" data-testid="opened-files-tabs">
          <TabStrip />
        </div>
        <div class="min-h-0 flex-1">
          <MainView>
            <template #empty>
              <EmptyState
                fill
                icon="file"
                :title="t('space.openedFiles.window.empty.title')"
                :description="t('space.openedFiles.window.empty.body')"
                data-testid="opened-files-empty"
              />
            </template>
          </MainView>
        </div>
      </section>
    </div>
  </div>
</template>
