<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { listen, type UnlistenFn } from '@tauri-apps/api/event'
import { isTauri } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { ChevronRight } from 'lucide-vue-next'
import { useUpdaterStore } from '@/stores/updater'
import UpdateNoticeDialog from './UpdateNoticeDialog.vue'

const updater = useUpdaterStore()
const isMainWindow = !isTauri() || getCurrentWindow().label === 'main'
let unlisten: UnlistenFn | undefined

const showPrompt = computed(() => {
  const version = updater.checkResult?.latest_version
  return Boolean(
    version
    && updater.checkResult?.has_update
    && !updater.dialogOpen
    && updater.progress.state === 'idle'
    && updater.dismissedVersion !== version
  )
})

onMounted(async () => {
  if (!isMainWindow) return
  if (isTauri()) {
    try {
      unlisten = await listen('check-update', () => { void updater.checkAndOpen() })
    }
    catch (error) {
      console.warn('[Updater] 监听菜单检查更新失败:', error)
    }
  }
  await updater.initialize()
})

onUnmounted(() => { unlisten?.() })
</script>

<template>
  <template v-if="isMainWindow">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="translate-y-2 opacity-0"
      leave-active-class="transition duration-150 ease-in"
      leave-to-class="translate-y-2 opacity-0"
    >
      <div v-if="showPrompt" class="pointer-events-none fixed inset-x-0 bottom-24 z-50 flex justify-center px-4">
        <button
          type="button"
          class="liquid-glass pointer-events-auto flex items-center gap-2 rounded-full border px-4 py-2 text-xs text-slate-700 shadow-lg transition-colors hover:text-slate-950 dark:text-slate-200 dark:hover:text-white"
          @click="updater.openDialog"
        >
          <span class="size-1.5 rounded-full bg-primary" />
          发现新版本
          <span class="font-mono font-medium">{{ updater.checkResult?.latest_version }}</span>
          <ChevronRight class="size-3.5 opacity-50" />
        </button>
      </div>
    </Transition>

    <UpdateNoticeDialog />
  </template>
</template>
