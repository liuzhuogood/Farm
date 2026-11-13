<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Plug } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import CopyValueButton from '@/components/common/CopyValueButton.vue'
import { fetchMcpConfiguration, setMcpInstallation, type McpClient } from '@/api/mcp'

const clients = ref<McpClient[]>([])
const endpoint = ref('')
const loading = ref(false)
const busy = ref('')
const error = ref('')
const notice = ref('')
const expanded = ref('')
let alive = true
let sequence = 0
onUnmounted(() => { alive = false; sequence++ })
const labels = { installed: '已安装', not_installed: '未安装', conflict: '配置冲突', error: '配置异常' }
async function load() {
  if (busy.value)
    return
  const seq = ++sequence
  loading.value = true
  error.value = ''
  try {
    const data = await fetchMcpConfiguration()
    if (alive && seq === sequence) {
      clients.value = data.clients
      endpoint.value = data.endpoint
    }
  }
  catch (err) {
    if (alive && seq === sequence)
      error.value = err instanceof Error ? err.message : '读取配置失败'
  }
  finally {
    if (alive && seq === sequence)
      loading.value = false
  }
}
async function mutate(client: McpClient) {
  if (busy.value || loading.value)
    return
  sequence++
  busy.value = client.id
  error.value = ''
  notice.value = ''
  try {
    const result = await setMcpInstallation(client.id, client.status !== 'installed')
    if (!alive)
      return
    clients.value = clients.value.map(item => item.id === result.id ? result : item)
    notice.value = `${result.name} ${result.status === 'installed' ? '已安装' : '已卸载'}`
  }
  catch (err) {
    if (alive)
      error.value = err instanceof Error ? err.message : '更新配置失败'
  }
  finally {
    if (alive)
      busy.value = ''
  }
}
onMounted(load)
</script>

<template>
  <section class="rounded-3xl border border-border/60 bg-card/80 p-6 shadow-sm" aria-label="MCP 接入">
    <div class="mb-5 flex items-center justify-between gap-4">
      <div>
        <h2 class="text-base font-semibold tracking-tight text-foreground sm:text-lg">
          MCP 接入
        </h2>
        <div v-if="endpoint" class="mt-1 flex items-center gap-0.5">
          <p class="break-all font-mono text-xs text-muted-foreground">{{ endpoint }}</p>
          <CopyValueButton :value="endpoint" label="服务地址" />
        </div>
      </div>
      <Button variant="outline" size="sm" :disabled="loading || !!busy" @click="load">刷新</Button>
    </div>
    <p v-if="loading" class="text-sm text-muted-foreground">正在读取配置…</p>
    <p v-if="error" role="alert" class="mb-3 text-sm text-destructive">{{ error }}</p>
    <p v-if="notice" role="status" class="mb-3 text-sm text-green-600">{{ notice }}</p>
    <div v-if="!loading && clients.length > 0" class="space-y-3">
      <article v-for="client in clients" :key="client.id" class="rounded-2xl border border-border/60 bg-background/60 p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="flex items-center gap-2.5">
              <h3 class="font-medium">{{ client.name }}</h3>
              <Badge
                variant="outline"
                class="font-normal"
                :class="client.status === 'installed' ? 'border-emerald-500/40 text-emerald-600 dark:text-emerald-400' : ''"
              >
                {{ labels[client.status] }}
              </Badge>
            </div>
            <p class="mt-1 break-all text-xs text-muted-foreground">{{ client.path }}</p>
          </div>
          <div class="flex gap-2">
            <Button variant="ghost" size="sm" :aria-expanded="expanded === client.id" @click="expanded = expanded === client.id ? '' : client.id">配置</Button>
            <Button :variant="client.status === 'installed' ? 'outline' : 'default'" size="sm" :disabled="loading || !!busy || client.status === 'conflict' || client.status === 'error'" @click="mutate(client)">
              {{ busy === client.id ? '处理中…' : client.status === 'installed' ? '卸载' : '安装' }}
            </Button>
          </div>
        </div>
        <p v-if="client.error" class="mt-2 text-sm text-destructive">{{ client.error }}</p>
        <pre v-if="expanded === client.id" class="mt-3 overflow-x-auto rounded-xl bg-muted/60 p-3 text-xs">{{ client.config }}</pre>
      </article>
    </div>
    <Empty v-else-if="!loading" class="border-0 py-8">
      <EmptyMedia>
        <Plug class="h-8 w-8 text-muted-foreground" />
      </EmptyMedia>
      <EmptyHeader>
        <EmptyTitle class="text-base">未发现 MCP 客户端</EmptyTitle>
        <EmptyDescription class="text-xs">当前宿主机没有可配置的 MCP 客户端。</EmptyDescription>
      </EmptyHeader>
    </Empty>
  </section>
</template>
