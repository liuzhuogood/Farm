<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRouter, RouterView } from 'vue-router'
import { useRoute } from 'vue-router'
import { isTauri } from '@tauri-apps/api/core'
import { listen, type UnlistenFn } from '@tauri-apps/api/event'
import { getCurrentWindow } from '@tauri-apps/api/window'
import ApprovalPopup from '@/components/ai-integration/ApprovalPopup.vue'
import AppShell from '@/components/layout/AppShell.vue'
import { Toaster } from '@/components/ui/sonner'
import UpdateCenter from '@/components/updater/UpdateCenter.vue'

const router = useRouter()
const route = useRoute()
const embeddedRoute = computed(() => route.query.embedded === '1' || route.query.embedded === 'true')
const standaloneWindow = (() => {
  if (!isTauri()) return false
  const label = getCurrentWindow().label
  return label.startsWith('player-') || label.startsWith('safety-approval-')
})()
const useShell = computed(() => !embeddedRoute.value && !standaloneWindow && route.meta.shell !== false)
let unlisten: UnlistenFn | undefined

onMounted(async () => {
  if (typeof window !== 'undefined') {
    (window as unknown as { __NAVIGATE_TO_PLAYER__?: (serial: string) => void }).__NAVIGATE_TO_PLAYER__ = (serial: string) => {
      router.push({ name: 'player', query: { serial } })
    }
  }

  if (isTauri()) {
    try {
      unlisten = await listen<string>('navigate-to-player', (event) => {
        if (event.payload) {
          router.push({ name: 'player', query: { serial: event.payload } })
        }
      })
    }
    catch (e) {
      console.warn('[App] 监听导航事件失败:', e)
    }
  }
})

onUnmounted(() => {
  if (unlisten) {
    unlisten()
  }
})
</script>

<template>
  <AppShell v-if="useShell">
    <RouterView />
  </AppShell>
  <RouterView v-else />
  <ApprovalPopup />
  <UpdateCenter />
  <Toaster position="bottom-center" />
</template>
