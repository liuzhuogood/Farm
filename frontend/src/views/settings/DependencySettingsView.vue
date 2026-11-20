<script setup lang="ts">
import DependencyPanel from '@/components/dependencies/DependencyPanel.vue'
import MatchingServicePanel from '@/components/system/MatchingServicePanel.vue'
import OcrServicePanel from '@/components/system/OcrServicePanel.vue'
import SettingsPage from '@/components/system/SettingsPage.vue'
</script>

<template>
  <SettingsPage title="依赖" description="内置依赖与 OCR、模板匹配服务。修改后自动保存并生效。">
    <DependencyPanel :active="true" class="rounded-xl border bg-card">
      <MatchingServicePanel />
      <OcrServicePanel />
    </DependencyPanel>
  </SettingsPage>
</template>
