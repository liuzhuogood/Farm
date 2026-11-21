<script setup lang="ts">
import { computed } from 'vue'
import { invoke, isTauri } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { useRoute } from 'vue-router'
import AppSidebar from './AppSidebar.vue'

const route = useRoute()
const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPod|iPad/i.test(navigator.userAgent || navigator.platform)
const showDragRegion = computed(() => isTauri() && isMac && route.meta.pageHeader !== true)
let lastToggleTime = 0

async function toggleMaximize() {
  const now = Date.now()
  if (now - lastToggleTime < 400) return
  lastToggleTime = now
  try {
    await invoke('toggle_window_maximize')
    return
  }
  catch (cause) {
    console.warn('[Desktop] invoke toggle_window_maximize 失败', cause)
  }
  try {
    await getCurrentWindow().toggleMaximize()
  }
  catch (cause) {
    console.warn('[Desktop] 切换窗口最大化失败', cause)
  }
}

function handleDragMouseDown(event: MouseEvent) {
  if (event.detail === 2) void toggleMaximize()
}

function handleDragDoubleClick(event: MouseEvent) {
  if ((event.target as HTMLElement).closest('button, a, input, select, textarea')) return
  void toggleMaximize()
}
</script>

<template>
  <div class="flex h-dvh w-full overflow-hidden bg-slate-50 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
    <AppSidebar />
    <div class="relative min-w-0 flex-1 overflow-hidden">
      <div
        v-if="showDragRegion"
        data-tauri-drag-region
        class="absolute inset-x-0 top-0 z-20 h-7 select-none"
        aria-hidden="true"
        @mousedown="handleDragMouseDown"
        @dblclick="handleDragDoubleClick"
      />
      <div class="h-full overflow-hidden" :class="showDragRegion ? 'pt-7' : ''">
        <slot />
      </div>
    </div>
  </div>
</template>
