<script setup lang="ts">
import { onMounted } from 'vue'
import { Monitor, Moon, RefreshCw, Sun } from 'lucide-vue-next'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import IdleScreenSettings from '@/components/system/IdleScreenSettings.vue'
import PlayerSettingsContent from '@/components/system/PlayerSettingsContent.vue'
import SettingsPage from '@/components/system/SettingsPage.vue'
import { theme } from '@/composables/useTheme'
import { useUpdateSettings } from '@/composables/useUpdateSettings'
import { useUpdaterStore } from '@/stores/updater'

const { settings: updateSettings, channel: updateChannel, loadChannel, changeChannel, setCheckOnStartup, setAutoUpdate } = useUpdateSettings()
const updater = useUpdaterStore()
const themes = [
  { id: 'auto' as const, name: '系统', icon: Monitor },
  { id: 'light' as const, name: '明亮', icon: Sun },
  { id: 'dark' as const, name: '黑暗', icon: Moon },
]

onMounted(() => { void loadChannel() })

async function checkNow() {
  if (updater.isChecking) return
  await updater.checkAndOpen()
}

async function handleChannel(value: 'stable' | 'beta') {
  if (value === updateChannel.value) return
  try {
    await changeChannel(value)
    await updater.check()
  }
  catch (cause) {
    console.error('[更新通道] 切换失败', cause)
    toast.error('切换更新通道失败')
  }
}
</script>

<template>
  <SettingsPage title="通用设置" description="外观、更新、空闲熄屏与投屏默认参数。">
    <div class="space-y-6">
      <section class="space-y-3 rounded-xl border bg-card p-4">
        <h2 class="text-sm font-semibold">外观</h2>
        <div class="grid grid-cols-3 gap-3" role="group" aria-label="主题">
          <Button
            v-for="item in themes"
            :key="item.id"
            variant="outline"
            class="h-24 flex-col gap-3"
            :class="theme === item.id ? 'border-primary bg-primary/5 text-primary' : ''"
            :aria-pressed="theme === item.id"
            @click="theme = item.id"
          >
            <component :is="item.icon" class="size-6" />
            {{ item.name }}
          </Button>
        </div>
      </section>

      <section class="space-y-3 rounded-xl border bg-card p-4">
        <h2 class="text-sm font-semibold">更新</h2>
        <div class="divide-y rounded-lg border">
          <div class="flex items-center justify-between gap-4 px-4 py-3">
            <label for="update-check" class="text-sm">检查更新</label>
            <div class="flex items-center gap-2">
              <Select :model-value="updateChannel" @update:model-value="value => handleChannel(value as 'stable' | 'beta')">
                <SelectTrigger class="w-32" aria-label="更新通道">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="stable">正式版</SelectItem>
                  <SelectItem value="beta">beta 测试版</SelectItem>
                </SelectContent>
              </Select>
              <Button variant="ghost" size="icon-sm" aria-label="立即检查更新" :disabled="updater.isChecking" @click="checkNow">
                <RefreshCw class="size-4" :class="{ 'animate-spin': updater.isChecking }" />
              </Button>
              <Switch id="update-check" :model-value="updateSettings.checkOnStartup" aria-label="检查更新" @update:model-value="value => setCheckOnStartup(value === true)" />
            </div>
          </div>
          <div class="flex items-center justify-between gap-4 px-4 py-3">
            <label for="update-auto" class="text-sm" :class="{ 'text-muted-foreground': !updateSettings.checkOnStartup }">自动更新</label>
            <Switch id="update-auto" :model-value="updateSettings.autoUpdate" :disabled="!updateSettings.checkOnStartup" aria-label="自动更新" @update:model-value="value => setAutoUpdate(value === true)" />
          </div>
        </div>
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold">设备与电源</h2>
        <IdleScreenSettings />
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold">投屏设置</h2>
        <PlayerSettingsContent />
      </section>
    </div>
  </SettingsPage>
</template>
