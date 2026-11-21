<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const enabled = ref(true)
const timeoutMinutes = ref(10)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
let request: AbortController | undefined
let disposed = false

const selectedValue = computed(() => enabled.value ? String(timeoutMinutes.value) : 'off')

async function update(next?: { enabled: boolean, timeoutMinutes: number }) {
  if (saving.value || disposed) return
  request?.abort()
  const controller = new AbortController()
  request = controller
  const timeout = window.setTimeout(() => controller.abort(), 10000)
  const isSave = next !== undefined
  saving.value = isSave
  loading.value = !isSave
  error.value = ''
  try {
    const response = await fetch('/api/system/idle-screen', {
      method: isSave ? 'PUT' : 'GET',
      headers: isSave ? { 'Content-Type': 'application/json' } : undefined,
      body: isSave ? JSON.stringify({ enabled: next.enabled, timeout_minutes: next.timeoutMinutes }) : undefined,
      signal: controller.signal,
    })
    if (!response.ok) throw new Error((await response.text()).trim() || '熄屏设置请求失败')
    const config: { enabled: boolean, timeout_minutes: number } = await response.json()
    if (typeof config.enabled !== 'boolean' || typeof config.timeout_minutes !== 'number') throw new Error('熄屏设置数据无效')
    if (!disposed && request === controller) {
      enabled.value = config.enabled
      timeoutMinutes.value = config.timeout_minutes
    }
  }
  catch (cause) {
    if (!disposed && request === controller) error.value = cause instanceof Error ? cause.message : '熄屏设置请求失败'
  }
  finally {
    window.clearTimeout(timeout)
    if (!disposed && request === controller) {
      loading.value = false
      saving.value = false
      request = undefined
    }
  }
}

function change(value: unknown) {
  const selected = String(value ?? 'off')
  void update({
    enabled: selected !== 'off',
    timeoutMinutes: selected === 'off' ? timeoutMinutes.value : Number(selected),
  })
}

onMounted(() => { void update() })
onUnmounted(() => { disposed = true; request?.abort() })
</script>

<template>
  <div class="space-y-3 rounded-lg border p-4">
    <div class="flex items-center justify-between gap-4">
      <div>
        <p id="idle-screen-label" class="text-sm font-medium">空闲自动熄屏</p>
        <p id="idle-screen-description" class="text-xs text-muted-foreground">
          {{ enabled ? `设备无投屏、ADB 等操作满 ${timeoutMinutes} 分钟后自动熄屏` : '不会因空闲自动熄屏' }}
        </p>
      </div>
      <Select
        :model-value="selectedValue"
        :disabled="loading || saving"
        @update:model-value="change"
      >
        <SelectTrigger class="w-36" aria-label="空闲自动熄屏时间">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="off">不熄屏</SelectItem>
          <SelectItem value="1">1 分钟</SelectItem>
          <SelectItem value="5">5 分钟</SelectItem>
          <SelectItem value="10">10 分钟</SelectItem>
          <SelectItem value="30">30 分钟</SelectItem>
        </SelectContent>
      </Select>
    </div>
    <div v-if="error" class="flex items-center justify-between gap-3" role="alert">
      <p class="text-xs text-destructive">{{ error }}</p>
      <Button variant="outline" size="sm" @click="update()">重新加载</Button>
    </div>
  </div>
</template>
