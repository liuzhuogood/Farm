<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { invoke, isTauri } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import DevicePlayer from '@/components/player/DevicePlayer.vue'
import FloatingPlayer from '@/components/player/FloatingPlayer.vue'
import { useFloatingPlayers } from '@/composables/useFloatingPlayers'

const { floatingSerials, close: closeFloatingWindow } = useFloatingPlayers()

const route = useRoute()
const serial = computed(() => typeof route.query.serial === 'string' ? route.query.serial : '')
const embedded = computed(() => route.query.embedded === '1' || route.query.embedded === 'true')

watch(embedded, value => {
  document.documentElement.classList.toggle('desktop-player', value && isTauri() && getCurrentWindow().label.startsWith('player-'))
}, { immediate: true })
onUnmounted(() => document.documentElement.classList.remove('desktop-player'))

let layout: { width: number, height: number } | undefined
let fitTimer: ReturnType<typeof setTimeout> | undefined
let fitting = false
let disposed = false
function queueFit() {
  clearTimeout(fitTimer)
  fitTimer = setTimeout(() => { void fitNative() }, 80)
}
async function fitNative() {
  if (disposed || fitting || !layout || !embedded.value || !isTauri() || !getCurrentWindow().label.startsWith('player-')) return
  fitting = true
  try { await invoke('fit_player_window', { videoWidth: layout.width, videoHeight: layout.height }) }
  catch (cause) { console.warn('[Desktop] 适配视频小窗失败', cause) }
  finally { fitting = false }
}
function handleLayout(next: {width: number, height: number}) {
  if (next.width <= 0 || next.height <= 0) return
  layout = next
  queueFit()
}
onMounted(() => window.addEventListener('resize', queueFit))
onUnmounted(() => { disposed = true; clearTimeout(fitTimer); window.removeEventListener('resize', queueFit) })
async function handleClose() {
  if (isTauri()) {
    try {
      await invoke('close_window')
      return
    }
    catch (e) {
      console.warn('[Desktop] invoke close_window 失败，尝试 window.close()', e)
    }
    try {
      await getCurrentWindow().close()
    }
    catch (e) {
      console.warn('[Desktop] 关闭窗口失败', e)
    }
  }
}
</script>

<template>
  <div class="h-full w-full overflow-hidden">
    <DevicePlayer
      :key="serial"
      :serial="serial"
      :embedded="embedded"
      @close="handleClose"
      @layout="handleLayout"
    />

    <!-- Web 端内嵌投屏小窗（与首页共用同一份浮窗列表） -->
    <FloatingPlayer
      v-for="(floatingSerial, index) in floatingSerials"
      :key="floatingSerial"
      :serial="floatingSerial"
      :offset-index="index"
      :z-index="60 + index"
      @close="closeFloatingWindow(floatingSerial)"
    />
  </div>
</template>
