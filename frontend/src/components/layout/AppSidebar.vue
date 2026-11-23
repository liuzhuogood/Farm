<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { invoke, isTauri } from '@tauri-apps/api/core'
import {
  Bot,
  Braces,
  ChevronLeft,
  ChevronRight,
  Info,
  ListChecks,
  Loader2,
  MessageSquare,
  Package,
  RefreshCw,
  RotateCcw,
  Server,
  Settings,
  SlidersHorizontal,
  Smartphone,
} from 'lucide-vue-next'
import { toast } from 'vue-sonner'

const route = useRoute()
const collapsed = ref(localStorage.getItem('farm.sidebar.collapsed') === '1')
const settingsOpen = ref(route.path.startsWith('/settings'))
const restoring = ref(false)
const restarting = ref(false)

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPod|iPad/i.test(navigator.userAgent || navigator.platform)
const isMacDesktop = computed(() => isTauri() && isMac)
const desktop = isTauri()
const appIconUrl = `${import.meta.env.BASE_URL}app-icon.png`

const nav = [
  { label: '设备列表', to: '/', icon: Smartphone, match: (path: string) => path === '/' },
  { label: '任务记录', to: '/tasks', icon: ListChecks, match: (path: string) => path.startsWith('/tasks') },
  { label: 'AI 集成', to: '/ai-integration', icon: Bot, match: (path: string) => path.startsWith('/ai-integration') },
  { label: 'API', to: '/api', icon: Braces, match: (path: string) => path.startsWith('/api') && path !== '/api-docs.html' },
]

const settingsNav = [
  { label: '通用设置', to: '/settings/general', icon: SlidersHorizontal },
  { label: '服务', to: '/settings/service', icon: Server },
  { label: '依赖', to: '/settings/dependencies', icon: Package },
  { label: '反馈', to: '/settings/feedback', icon: MessageSquare },
  { label: '关于', to: '/settings/about', icon: Info },
]

const settingsActive = computed(() => route.path.startsWith('/settings'))

watch(() => route.path, (path) => {
  if (path.startsWith('/settings')) settingsOpen.value = true
})

function toggleCollapsed() {
  collapsed.value = !collapsed.value
  localStorage.setItem('farm.sidebar.collapsed', collapsed.value ? '1' : '0')
  if (collapsed.value) settingsOpen.value = false
}

function toggleSettings() {
  if (collapsed.value) {
    collapsed.value = false
    localStorage.setItem('farm.sidebar.collapsed', '0')
    settingsOpen.value = true
    return
  }
  settingsOpen.value = !settingsOpen.value
}

function authHeaders(): Record<string, string> {
  const token = (window as Window & { __FARM_DESKTOP_TOKEN__?: string }).__FARM_DESKTOP_TOKEN__
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function restoreAllPlayers() {
  if (!desktop || restoring.value) return
  restoring.value = true
  try {
    await invoke('restore_all_player_windows')
    toast.success('已恢复所有小窗')
  }
  catch (cause) {
    console.error('[Sidebar] 恢复所有小窗失败', cause)
    toast.error(cause instanceof Error ? cause.message : '恢复所有小窗失败')
  }
  finally {
    restoring.value = false
  }
}

async function restartAdb() {
  if (restarting.value) return
  restarting.value = true
  try {
    const response = await fetch('/api/adb/restart', { method: 'POST', headers: authHeaders() })
    if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`)
    toast.success('已请求重启 ADB 服务')
  }
  catch (cause) {
    console.error('[Sidebar] 重启 ADB 服务失败', cause)
    toast.error(cause instanceof Error ? cause.message : '重启 ADB 服务失败')
  }
  finally {
    restarting.value = false
  }
}
</script>

<template>
  <aside
    class="relative z-30 flex h-full shrink-0 flex-col border-r border-slate-200/80 bg-white/85 backdrop-blur-2xl transition-[width] duration-200 dark:border-white/10 dark:bg-slate-950/85"
    :class="collapsed ? 'w-16' : 'w-[236px]'"
    aria-label="主导航"
  >
    <div
      data-tauri-drag-region
      class="flex h-11 shrink-0 items-center gap-2"
      :class="collapsed ? 'justify-center px-2' : isMacDesktop ? 'pl-[84px] pr-2' : 'px-3'"
    >
      <RouterLink v-if="!collapsed" to="/" class="flex min-w-0 items-center gap-2">
        <img :src="appIconUrl" alt="" class="size-7 shrink-0 object-contain" width="28" height="28">
        <span class="truncate text-sm font-bold tracking-tight">Farm</span>
      </RouterLink>
      <button
        v-if="!collapsed"
        type="button"
        class="ml-auto grid size-7 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-white/10 dark:hover:text-white"
        aria-label="收起菜单"
        title="收起菜单"
        @click="toggleCollapsed"
      >
        <ChevronLeft class="size-4" />
      </button>
    </div>

    <button
      v-if="collapsed"
      type="button"
      class="mx-auto mt-2 grid size-8 place-items-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-white/10 dark:hover:text-white"
      aria-label="展开菜单"
      title="展开菜单"
      @click="toggleCollapsed"
    >
      <ChevronRight class="size-4" />
    </button>

    <nav class="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto px-2 py-3">
      <RouterLink
        v-for="item in nav"
        :key="item.to"
        :to="item.to"
        class="group flex h-10 items-center gap-3 rounded-xl px-3 text-sm font-medium transition-colors"
        :class="[
          item.match(route.path)
            ? 'bg-blue-600 text-white shadow-sm shadow-blue-600/20'
            : 'text-slate-600 hover:bg-slate-100 hover:text-slate-950 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-white',
          collapsed ? 'justify-center px-0' : '',
        ]"
        :title="collapsed ? item.label : undefined"
      >
        <component :is="item.icon" class="size-[18px] shrink-0" />
        <span v-if="!collapsed" class="truncate">{{ item.label }}</span>
      </RouterLink>
    </nav>

    <div class="shrink-0 border-t border-slate-200/70 p-2 dark:border-white/10">
      <div v-if="settingsOpen && !collapsed" class="mb-2 space-y-1 rounded-xl bg-slate-50 p-1.5 dark:bg-white/[0.04]">
        <p class="px-2 pt-1 text-[10px] font-semibold uppercase tracking-wider text-slate-400 dark:text-slate-500">设置</p>
        <RouterLink
          v-for="item in settingsNav"
          :key="item.to"
          :to="item.to"
          class="flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-xs font-medium transition-colors"
          :class="route.path === item.to
            ? 'bg-white text-blue-600 shadow-sm dark:bg-white/10 dark:text-blue-300'
            : 'text-slate-600 hover:bg-white hover:text-slate-950 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-white'"
        >
          <component :is="item.icon" class="size-4 shrink-0" />
          {{ item.label }}
        </RouterLink>
        <div class="my-1 border-t border-slate-200/70 dark:border-white/10" />
        <button
          v-if="desktop"
          type="button"
          class="flex h-9 w-full items-center gap-2.5 rounded-lg px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-white hover:text-slate-950 disabled:opacity-50 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-white"
          :disabled="restoring"
          @click="restoreAllPlayers"
        >
          <Loader2 v-if="restoring" class="size-4 shrink-0 animate-spin" />
          <RotateCcw v-else class="size-4 shrink-0" />
          恢复所有小窗
        </button>
        <button
          type="button"
          class="flex h-9 w-full items-center gap-2.5 rounded-lg px-2.5 text-xs font-medium text-slate-600 transition-colors hover:bg-white hover:text-slate-950 disabled:opacity-50 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-white"
          :disabled="restarting"
          @click="restartAdb"
        >
          <Loader2 v-if="restarting" class="size-4 shrink-0 animate-spin" />
          <RefreshCw v-else class="size-4 shrink-0" />
          重启 ADB 服务
        </button>
      </div>

      <button
        type="button"
        class="flex h-10 w-full items-center gap-3 rounded-xl px-3 text-sm font-medium transition-colors"
        :class="[
          settingsActive || settingsOpen
            ? 'bg-slate-100 text-slate-950 dark:bg-white/10 dark:text-white'
            : 'text-slate-600 hover:bg-slate-100 hover:text-slate-950 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-white',
          collapsed ? 'justify-center px-0' : '',
        ]"
        :title="collapsed ? '设置' : undefined"
        :aria-expanded="settingsOpen"
        @click="toggleSettings"
      >
        <Settings class="size-[18px] shrink-0" />
        <span v-if="!collapsed" class="truncate">设置</span>
        <ChevronRight
          v-if="!collapsed"
          class="ml-auto size-4 transition-transform"
          :class="settingsOpen ? 'rotate-90' : ''"
        />
      </button>
    </div>
  </aside>
</template>
