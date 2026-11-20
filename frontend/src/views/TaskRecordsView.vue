<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { CheckCircle2, CircleAlert, Clock3, Image, ListChecks, Loader2, PackageOpen, Search, X } from 'lucide-vue-next'
import { fetchAutomationTask, listAllTaskRecords, type AutomationTask, type TaskRecordSummary } from '@/api/automationTasks'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useDeviceStore } from '@/stores/devices'

const store = useDeviceStore()
const tasks = ref<TaskRecordSummary[]>([])
const loading = ref(true)
const error = ref('')
const filter = ref<'all' | 'running' | 'done' | 'failed' | 'stopped'>('all')
const keyword = ref('')
const selected = ref<TaskRecordSummary | null>(null)
const detail = ref<AutomationTask | null>(null)
const detailLoading = ref(false)
const detailError = ref('')
let alive = true
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let detailController: AbortController | undefined
let polling = false

const states = [
  { id: 'all', label: '全部' },
  { id: 'running', label: '进行中' },
  { id: 'done', label: '已完成' },
  { id: 'failed', label: '失败' },
  { id: 'stopped', label: '已停止' },
] as const
const stateLabel = { running: '进行中', done: '已完成', failed: '失败', stopped: '已停止' } as const
const stepLabel = { pending: '待执行', running: '执行中', done: '已完成', failed: '失败', skipped: '已跳过' } as const

const filtered = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  return tasks.value.filter(task => (filter.value === 'all' || task.state === filter.value)
    && (!value || [task.title, task.serial, deviceName(task.serial), task.summary].join(' ').toLowerCase().includes(value)))
})

function deviceName(serial: string) {
  const device = store.devices.find(item => item.serial === serial)
  return device ? store.displayName(device) : serial
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function duration(task: TaskRecordSummary) {
  const start = Date.parse(task.started_at)
  const end = task.ended_at ? Date.parse(task.ended_at) : Date.now()
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return ''
  const seconds = Math.round((end - start) / 1000)
  if (seconds < 60) return `${seconds} 秒`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分 ${seconds % 60} 秒`
  return `${Math.floor(minutes / 60)} 时 ${minutes % 60} 分`
}

function schedule() {
  clearTimeout(timer)
  if (alive && document.visibilityState !== 'hidden') timer = setTimeout(() => { void refresh() }, 2500)
}

async function refresh() {
  if (!alive || polling) return
  polling = true
  const request = new AbortController()
  controller = request
  try {
    const next = await listAllTaskRecords(request.signal)
    if (!alive) return
    tasks.value = next
    error.value = ''
    if (selected.value) {
      const current = next.find(task => task.id === selected.value!.id)
      if (!current) {
        selected.value = null
        detail.value = null
      }
      else {
        selected.value = current
        if (detail.value?.state === 'running') void loadDetail(current)
      }
    }
  }
  catch (cause) {
    if (!alive) return
    error.value = cause instanceof Error ? cause.message : '读取任务记录失败'
  }
  finally {
    if (alive) {
      polling = false
      loading.value = false
      controller = undefined
      schedule()
    }
  }
}

async function loadDetail(task: TaskRecordSummary) {
  detailController?.abort()
  const request = new AbortController()
  detailController = request
  detailLoading.value = true
  detailError.value = ''
  try {
    const next = await fetchAutomationTask(task.serial, task.id, request.signal)
    if (alive && selected.value?.id === task.id) detail.value = next
  }
  catch (cause) {
    if (alive && selected.value?.id === task.id) detailError.value = cause instanceof Error ? cause.message : '读取任务详情失败'
  }
  finally {
    if (alive && selected.value?.id === task.id) detailLoading.value = false
  }
}

function choose(task: TaskRecordSummary) {
  selected.value = task
  detail.value = null
  void loadDetail(task)
}

function closeDetail() {
  detailController?.abort()
  selected.value = null
  detail.value = null
  detailError.value = ''
}

function onVisibility() {
  if (document.visibilityState === 'hidden') {
    clearTimeout(timer)
  }
  else {
    void refresh()
  }
}

onMounted(() => {
  store.startPolling()
  document.addEventListener('visibilitychange', onVisibility)
  void refresh()
})

onUnmounted(() => {
  alive = false
  clearTimeout(timer)
  controller?.abort()
  detailController?.abort()
  store.stopPolling()
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <div class="flex h-full min-h-0">
    <div class="min-w-0 flex-1 overflow-y-auto">
      <main class="mx-auto w-full max-w-6xl px-4 py-6 sm:px-8">
        <header class="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h1 class="text-2xl font-bold tracking-tight">任务记录</h1>
            <p class="mt-1 text-sm text-muted-foreground">所有设备的自动化任务与 MCP 历史记录。</p>
          </div>
          <Button variant="outline" size="sm" :disabled="loading" @click="refresh">
            <Loader2 v-if="loading" class="mr-1.5 size-3.5 animate-spin" />
            <ListChecks v-else class="mr-1.5 size-3.5" />
            刷新
          </Button>
        </header>

        <div class="mt-5 flex flex-wrap items-center gap-2">
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="item in states"
              :key="item.id"
              type="button"
              class="h-8 rounded-full px-3 text-xs font-medium transition-colors"
              :class="filter === item.id ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-white/10 dark:text-slate-300 dark:hover:bg-white/15'"
              @click="filter = item.id"
            >
              {{ item.label }}
            </button>
          </div>
          <div class="relative ml-auto w-full sm:w-64">
            <Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input v-model="keyword" placeholder="搜索任务或设备" class="h-8 pl-8" />
          </div>
        </div>

        <div v-if="error" role="alert" class="mt-4 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{{ error }}</div>

        <div v-if="loading && tasks.length === 0" class="mt-4 space-y-2">
          <div v-for="index in 6" :key="index" class="h-[72px] animate-pulse rounded-xl border bg-slate-100/80 dark:bg-white/5" />
        </div>

        <div v-else-if="filtered.length === 0" class="mt-10 flex flex-col items-center justify-center rounded-2xl border border-dashed px-6 py-16 text-center">
          <PackageOpen class="size-10 text-muted-foreground/60" />
          <p class="mt-3 text-sm font-medium">没有匹配的任务</p>
          <p class="mt-1 text-xs text-muted-foreground">{{ tasks.length === 0 ? '设备执行自动化任务后会显示在这里。' : '换一个状态或关键词试试。' }}</p>
        </div>

        <div v-else class="mt-4 overflow-hidden rounded-xl border bg-card">
          <button
            v-for="task in filtered"
            :key="task.id"
            type="button"
            class="flex w-full items-center gap-3 border-b px-4 py-3 text-left transition-colors last:border-b-0 hover:bg-muted/40"
            :class="selected?.id === task.id ? 'bg-blue-50/70 dark:bg-blue-500/10' : ''"
            @click="choose(task)"
          >
            <span class="grid size-9 shrink-0 place-items-center rounded-full" :class="task.state === 'running' ? 'bg-blue-500/10 text-blue-600' : task.state === 'failed' ? 'bg-red-500/10 text-red-600' : 'bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-300'">
              <Loader2 v-if="task.state === 'running'" class="size-4 animate-spin" />
              <CircleAlert v-else-if="task.state === 'failed'" class="size-4" />
              <CheckCircle2 v-else class="size-4" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-medium">{{ task.title || '未命名任务' }}</span>
              <span class="mt-0.5 block truncate text-xs text-muted-foreground">{{ deviceName(task.serial) }} · {{ task.source === 'mcp' ? 'MCP 记录' : '自动化' }} · {{ task.step_count }} 步</span>
            </span>
            <span class="hidden shrink-0 text-right text-xs text-muted-foreground sm:block">
              <span class="block">{{ formatTime(task.started_at) }}</span>
              <span class="mt-0.5 block">{{ duration(task) }}</span>
            </span>
            <Badge variant="outline" class="shrink-0" :class="task.state === 'failed' ? 'border-red-500/30 text-red-600' : task.state === 'running' ? 'border-blue-500/30 text-blue-600' : ''">
              {{ stateLabel[task.state] }}
            </Badge>
          </button>
        </div>
      </main>
    </div>

    <aside v-if="selected" class="flex w-full max-w-[440px] shrink-0 flex-col border-l bg-white dark:bg-slate-950 lg:w-[420px]">
      <header class="flex items-start gap-2 border-b px-4 py-3">
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-sm font-semibold">{{ selected.title || '未命名任务' }}</h2>
          <p class="mt-0.5 truncate text-xs text-muted-foreground">{{ deviceName(selected.serial) }} · {{ selected.serial }}</p>
        </div>
        <Button variant="ghost" size="icon-sm" aria-label="关闭详情" @click="closeDetail"><X class="size-4" /></Button>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto p-4">
        <div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="outline">{{ stateLabel[selected.state] }}</Badge>
          <span>{{ selected.source === 'mcp' ? 'MCP 记录' : '自动化任务' }}</span>
          <span>{{ formatTime(selected.started_at) }}</span>
          <span v-if="duration(selected)">{{ duration(selected) }}</span>
        </div>
        <p v-if="selected.summary" class="mt-3 rounded-lg bg-muted/50 px-3 py-2 text-xs">{{ selected.summary }}</p>

        <div v-if="detailLoading" class="mt-6 flex items-center justify-center gap-2 text-xs text-muted-foreground">
          <Loader2 class="size-4 animate-spin" />
          正在读取任务详情…
        </div>
        <p v-else-if="detailError" role="alert" class="mt-4 text-xs text-destructive">{{ detailError }}</p>
        <template v-else-if="detail">
          <h3 class="mt-5 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
            <Clock3 class="size-3.5" /> 步骤
            <span class="font-normal">（{{ detail.steps.length }}）</span>
          </h3>
          <div v-if="detail.steps.length === 0" class="mt-2 text-xs text-muted-foreground">暂无步骤记录。</div>
          <ol v-else class="mt-2 space-y-2">
            <li v-for="step in detail.steps" :key="step.index" class="rounded-lg border p-2.5">
              <div class="flex items-start justify-between gap-2">
                <p class="min-w-0 break-words text-xs font-medium">{{ step.index }}. {{ step.title }}</p>
                <Badge variant="outline" class="shrink-0 text-[10px]">{{ stepLabel[step.status] }}</Badge>
              </div>
              <pre class="mt-1.5 whitespace-pre-wrap break-all rounded bg-muted/50 px-2 py-1 text-[11px] text-muted-foreground">{{ step.command }}</pre>
              <p v-if="step.error" class="mt-1 text-[11px] text-destructive">{{ step.error }}</p>
              <p v-else-if="step.result" class="mt-1 line-clamp-4 whitespace-pre-wrap break-words text-[11px] text-muted-foreground">{{ step.result }}</p>
            </li>
          </ol>

          <h3 class="mt-5 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
            <Image class="size-3.5" /> 产物
            <span class="font-normal">（{{ detail.artifacts.length }}）</span>
          </h3>
          <div v-if="detail.artifacts.length === 0" class="mt-2 text-xs text-muted-foreground">暂无产物。</div>
          <div v-else class="mt-2 grid grid-cols-2 gap-2">
            <a
              v-for="artifact in detail.artifacts"
              :key="artifact.id"
              :href="artifact.download_url"
              target="_blank"
              rel="noopener"
              class="flex min-w-0 items-center gap-2 rounded-lg border p-2 text-xs transition-colors hover:bg-muted/50"
            >
              <span class="grid size-8 shrink-0 place-items-center rounded bg-muted"><Image v-if="artifact.kind === 'image'" class="size-4" /><PackageOpen v-else class="size-4" /></span>
              <span class="min-w-0 truncate">{{ artifact.relative_path.split('/').pop() }}</span>
            </a>
          </div>
        </template>
      </div>
    </aside>
  </div>
</template>
