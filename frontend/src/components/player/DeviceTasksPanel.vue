<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { Check, ChevronDown, Circle, Download, ExternalLink, File, Image, ListChecks, Loader2, RefreshCw, Square, Video, X } from 'lucide-vue-next'
import { defaultAutomationTask, fetchAutomationTask, listAutomationTasks, stopAutomationTask, taskDirectoryURL, type AutomationTask, type TaskArtifact, type TaskStep } from '@/api/automationTasks'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'

const props = withDefaults(defineProps<{ serial: string, active?: boolean }>(), { active: true })
const emit = defineEmits<{ close: [] }>()
const tasks = ref<AutomationTask[]>([])
const selected = ref<AutomationTask | null>(null)
const selectedId = ref('')
const detailId = ref('')
const detailLoadedId = ref('')
const followCurrent = ref(true)
const showAllSteps = ref(false)
const showAllArtifacts = ref(false)
const loading = ref(false)
const stopping = ref(false)
const error = ref('')
const previewErrors = ref(new Set<string>())
const now = ref(Date.now())
let alive = true
let generation = 0
let pollGeneration = 0
let inFlight = false
let controller: AbortController | undefined
let stopController: AbortController | undefined
let timer: ReturnType<typeof setTimeout> | undefined
const taskState = { running: '进行中', done: '已完成', failed: '失败', stopped: '已停止' } as const
const stepState = { pending: '待执行', running: '执行中', done: '已完成', failed: '失败', skipped: '已跳过' } as const
const completed = computed(() => selected.value?.steps.filter(step => step.status === 'done').length ?? 0)
const visibleSteps = computed(() => selected.value ? showAllSteps.value ? selected.value.steps : selected.value.steps.slice(-8) : [])
const visibleArtifacts = computed(() => selected.value ? showAllArtifacts.value ? selected.value.artifacts : selected.value.artifacts.slice(-4) : [])
const currentStep = computed(() => selected.value?.steps.find(step => step.status === 'running'))
const browseDirectory = computed(() => {
  const task = selected.value
  if (!task?.directory) return undefined
  return task.artifacts.some(artifact => !artifact.removed && (task.directory === '.' || artifact.relative_path.startsWith(`${task.directory}/`))) ? task.directory : undefined
})
const statusLine = computed(() => {
  const task = selected.value
  if (!task) return ''
  if (task.source === 'mcp' || task.state !== 'running') return ''
  if (!task.active) return '等待下一步指令'
  return currentStep.value ? `正在执行：${currentStep.value.title}` : '正在执行'
})

function canPoll() { return alive && props.active && Boolean(props.serial) && document.visibilityState !== 'hidden' && !stopping.value }
function cancelPoll() {
  pollGeneration++
  controller?.abort()
  controller = undefined
  clearTimeout(timer)
  timer = undefined
  inFlight = false
  loading.value = false
}
function resetContext() {
  generation++
  cancelPoll()
  stopController?.abort()
  stopController = undefined
  stopping.value = false
}
function schedule() {
  clearTimeout(timer)
  if (canPoll()) timer = setTimeout(() => { void refresh() }, 1000)
}
function choose(task: AutomationTask) {
  detailLoadedId.value = ''
  selected.value = task
  selectedId.value = task.id
  followCurrent.value = false
  showAllSteps.value = false
  showAllArtifacts.value = false
  previewErrors.value = new Set()
  detailId.value = task.id
  cancelPoll()
  void refresh()
}
function retry() {
  if (stopping.value) return
  cancelPoll()
  void refresh()
}
function loadEarlierSteps() {
  if (!selected.value) return
  followCurrent.value = false
  detailId.value = selected.value.id
  cancelPoll()
  void refresh()
}
function selectCurrent() {
  followCurrent.value = true
  detailId.value = ''
  detailLoadedId.value = ''
  updateSelection(tasks.value)
  cancelPoll()
  void refresh()
}
function updateSelection(next: AutomationTask[]) {
  if (followCurrent.value) {
    const current = defaultAutomationTask(next)
    if (current) {
      if (selectedId.value !== current.id) { showAllSteps.value = false; showAllArtifacts.value = false; previewErrors.value = new Set() }
      selectedId.value = current.id
      selected.value = current
    } else { selectedId.value = ''; selected.value = null }
  } else {
    const current = next.find(task => task.id === selectedId.value)
    // An explicitly selected old task remains visible if the recent list rolls over.
    if (current) selected.value = current
  }
}
async function refresh() {
  if (!canPoll() || inFlight) return
  const seq = generation
  const pollSeq = pollGeneration
  const serial = props.serial
  const request = new AbortController()
  controller = request
  inFlight = true
  loading.value = true
  let timedOut = false
  const timeout = setTimeout(() => { timedOut = true; request.abort() }, 8000)
  try {
    let next = await listAutomationTasks(serial, request.signal)
    if (!alive || seq !== generation || pollSeq !== pollGeneration || serial !== props.serial) return
    const requestedDetail = detailId.value
    let detail: AutomationTask | undefined
    if (requestedDetail) {
      // List and detail use the same cancellation budget and run sequentially.
      detail = await fetchAutomationTask(serial, requestedDetail, request.signal)
      if (!alive || seq !== generation || pollSeq !== pollGeneration || serial !== props.serial) return
      detailLoadedId.value = detail.id
      next = next.map(task => task.id === detail!.id ? detail! : task)
    }
    tasks.value = next
    updateSelection(next)
    if (detail && selectedId.value === detail.id) selected.value = detail
    error.value = ''
  } catch (cause) {
    if (!alive || seq !== generation || pollSeq !== pollGeneration) return
    error.value = timedOut ? '读取任务超时，请重试' : cause instanceof Error ? cause.message : '读取任务失败'
    console.warn('[DeviceTasks] 读取任务失败', { serial, error: error.value })
  } finally {
    clearTimeout(timeout)
    if (alive && seq === generation && pollSeq === pollGeneration) {
      inFlight = false
      loading.value = false
      controller = undefined
      now.value = Date.now()
      schedule()
    }
  }
}
async function stopTask() {
  const task = selected.value
  if (!task || task.source !== 'automation' || task.state !== 'running' || stopping.value) return
  cancelPoll()
  stopping.value = true
  const seq = generation
  const request = new AbortController()
  stopController = request
  const timeout = setTimeout(() => request.abort(), 8000)
  try {
    const finished = await stopAutomationTask(props.serial, task.id, request.signal)
    if (!alive || seq !== generation) return
    tasks.value = tasks.value.map(item => item.id === finished.id ? finished : item)
    if (selectedId.value === finished.id) selected.value = finished
    error.value = ''
  } catch (cause) {
    if (!alive || seq !== generation) return
    error.value = request.signal.aborted ? '停止任务超时，请刷新确认结果' : cause instanceof Error ? cause.message : '停止任务失败'
    console.warn('[DeviceTasks] 停止任务失败', { serial: props.serial, task: task.id, error: error.value })
  } finally {
    clearTimeout(timeout)
    if (alive && seq === generation) { stopping.value = false; stopController = undefined; schedule() }
  }
}
function visibilityChanged() {
  if (document.visibilityState === 'hidden') cancelPoll()
  else void refresh()
}
watch(() => props.serial, () => {
  resetContext()
  tasks.value = []
  selected.value = null
  selectedId.value = ''
  detailId.value = ''
  detailLoadedId.value = ''
  followCurrent.value = true
  stopping.value = false
  error.value = ''
  previewErrors.value = new Set()
  void refresh()
}, { immediate: true })
watch(() => props.active, active => { cancelPoll(); if (active) void refresh() })
document.addEventListener('visibilitychange', visibilityChanged)
onUnmounted(() => { alive = false; resetContext(); document.removeEventListener('visibilitychange', visibilityChanged) })
function formatTime(value: string) { return new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }) }
function elapsed(task: AutomationTask) {
  if (task.state !== 'running' && !task.ended_at) return ''
  const ms = (task.state === 'running' ? now.value : Date.parse(task.ended_at!)) - Date.parse(task.started_at)
  return duration(Math.max(0, ms))
}
function duration(ms: number) { return ms < 1000 ? `${ms} 毫秒` : ms < 60000 ? `${(ms / 1000).toFixed(1)} 秒` : `${Math.floor(ms / 60000)} 分 ${Math.floor(ms % 60000 / 1000)} 秒` }
function size(bytes: number) { return bytes < 1024 ? `${bytes} B` : bytes < 1024 * 1024 ? `${(bytes / 1024).toFixed(1)} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB` }
function markPreviewError(artifact: TaskArtifact) { previewErrors.value = new Set([...previewErrors.value, artifact.id]) }
function stepDuration(step: TaskStep) { return step.elapsed_ms > 0 ? duration(step.elapsed_ms) : '' }
</script>

<template>
  <section class="flex h-full min-h-0 flex-col bg-background" aria-label="设备任务" data-device-tasks>
    <div class="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-border px-4">
      <h2 class="flex items-center gap-2 text-sm font-semibold"><ListChecks class="size-4" />任务情况</h2>
      <div class="flex items-center gap-1">
        <Button variant="ghost" size="icon" class="size-7" aria-label="刷新任务" title="刷新任务" :disabled="stopping" @click="retry"><RefreshCw class="size-3.5" :class="{ 'animate-spin': loading }" /></Button>
        <Button variant="ghost" size="icon" class="size-7" aria-label="关闭任务面板" title="关闭任务面板" @click="emit('close')"><X class="size-4" /></Button>
      </div>
    </div>
    <ScrollArea class="min-h-0 flex-1">
      <div class="space-y-4 p-4">
        <div v-if="error" role="alert" class="rounded-xl border border-destructive/25 bg-destructive/5 p-3 text-sm">
          <p class="text-destructive">{{ error }}</p><Button variant="outline" size="sm" class="mt-2" :disabled="stopping" @click="retry">重试</Button>
        </div>
        <template v-if="loading && !selected && !error"><Skeleton class="h-28 rounded-xl" /><Skeleton class="h-40 rounded-xl" /></template>
        <Card v-else-if="!selected && !error" class="rounded-xl border-border bg-card shadow-none"><CardContent class="py-7 text-center text-sm text-muted-foreground">暂无任务</CardContent></Card>
        <template v-if="selected">
          <Card class="gap-0 rounded-xl border-border bg-card py-0 shadow-none" :data-task-id="selected.id">
            <CardContent class="space-y-3 p-4">
              <div class="flex items-center justify-between gap-2">
                <Badge :variant="selected.state === 'failed' ? 'destructive' : selected.state === 'done' ? 'default' : 'secondary'" class="gap-1.5 rounded-full">
                  <Loader2 v-if="selected.state === 'running' && selected.active" class="size-3 animate-spin" /><Circle v-else-if="selected.state === 'running'" class="size-2 fill-current" /><Check v-else-if="selected.state === 'done'" class="size-3" /> {{ selected.source === 'mcp' ? `记录状态：${taskState[selected.state]}` : taskState[selected.state] }}
                </Badge>
                <Button v-if="selected.source === 'automation' && selected.state === 'running'" variant="outline" size="sm" class="h-7 rounded-full px-2.5 text-xs" :disabled="stopping" @click="stopTask"><Square class="size-3" />{{ stopping ? '正在停止' : '停止任务' }}</Button>
              </div>
              <h3 class="break-words text-base font-semibold leading-snug">{{ selected.title }}</h3>
              <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"><span>{{ selected.source === 'mcp' ? 'MCP' : '自动化' }}</span><span>{{ formatTime(selected.started_at) }}</span><span v-if="elapsed(selected)">{{ elapsed(selected) }}</span></div>
              <p v-if="statusLine" class="break-words text-sm" role="status">{{ statusLine }}</p>
              <p v-if="!selected.steps_truncated" class="text-xs text-muted-foreground">累计完成 {{ completed }} 步<span v-if="selected.steps.length > completed"> · 已记录 {{ selected.steps.length }} 步</span></p>
              <p v-else class="text-xs text-muted-foreground">仅显示最近 {{ selected.steps.length }} 步</p>
              <p v-if="selected.summary" class="whitespace-pre-wrap break-words border-t border-border/50 pt-3 text-sm">{{ selected.summary }}</p>
            </CardContent>
          </Card>
          <section aria-label="执行步骤" class="space-y-2">
            <div class="flex items-center justify-between px-1"><h3 class="text-xs font-medium text-muted-foreground">执行步骤</h3><Button v-if="selected.steps.length > 8" variant="ghost" size="sm" class="h-6 text-xs" @click="showAllSteps = !showAllSteps">{{ showAllSteps ? '收起' : `${selected.steps_truncated ? '查看已加载' : '查看全部'} ${selected.steps.length} 步` }}</Button></div>
            <Button v-if="selected.steps_truncated && detailLoadedId !== selected.id" variant="outline" size="sm" class="w-full text-xs" :disabled="stopping" @click="loadEarlierSteps">加载较早步骤</Button>
            <p v-else-if="selected.steps_truncated" class="px-1 text-xs text-muted-foreground">已达到本次记录读取上限</p>
            <p v-if="!selected.steps.length" class="px-1 text-sm text-muted-foreground">{{ selected.source === 'automation' && selected.state === 'running' ? '等待第一条指令' : '暂无步骤记录' }}</p>
            <p v-if="!showAllSteps && selected.steps.length > 8" class="px-1 text-xs text-muted-foreground">此前 {{ selected.steps.length - 8 }} 步已收起</p>
            <ol class="space-y-2">
              <li v-for="step in visibleSteps" :key="`${selected.id}:${step.index}`" class="rounded-lg border border-border/70 bg-muted/20 p-3" :data-step-index="step.index">
                <details>
                  <summary class="flex cursor-pointer list-none items-start gap-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-md">
                    <Loader2 v-if="step.status === 'running'" class="mt-0.5 size-4 shrink-0 animate-spin text-primary" /><Check v-else-if="step.status === 'done'" class="mt-0.5 size-4 shrink-0 text-emerald-600" /><X v-else-if="step.status === 'failed'" class="mt-0.5 size-4 shrink-0 text-destructive" /><Circle v-else class="mt-1 size-3 shrink-0 text-muted-foreground" />
                    <span class="min-w-0 flex-1"><span class="block break-words" :class="step.status === 'running' ? 'font-semibold' : ''">{{ step.index }} · {{ step.title }}</span><span class="mt-1 block text-xs" :class="step.status === 'failed' ? 'text-destructive' : 'text-muted-foreground'">{{ stepState[step.status] }}<span v-if="stepDuration(step)"> · {{ stepDuration(step) }}</span></span></span><ChevronDown class="mt-1 size-3 shrink-0 text-muted-foreground" />
                  </summary>
                  <div class="mt-3 space-y-2 border-t border-border/50 pt-3 text-xs">
                    <pre class="whitespace-pre-wrap break-all rounded-xl bg-muted/60 p-2 font-mono" aria-label="执行指令">{{ step.command }}</pre>
                    <pre v-if="step.result" class="whitespace-pre-wrap break-all text-muted-foreground" aria-label="执行结果">{{ step.result }}</pre>
                    <p v-if="step.error" class="whitespace-pre-wrap break-all text-destructive" role="alert">{{ step.error }}</p>
                    <template v-for="artifact in step.artifacts" :key="artifact.id"><span v-if="artifact.removed" class="block break-all text-muted-foreground">{{ artifact.relative_path }} · 已删除</span><a v-else :href="artifact.download_url" class="flex items-center gap-1 text-primary underline" download><Download class="size-3" />{{ artifact.relative_path }}</a></template>
                  </div>
                </details>
              </li>
            </ol>
          </section>
          <section v-if="selected.artifacts.length || browseDirectory" aria-label="任务产物" class="space-y-2">
            <div class="flex items-center justify-between px-1"><h3 class="text-xs font-medium text-muted-foreground">任务产物</h3><a v-if="browseDirectory" :href="taskDirectoryURL(serial, browseDirectory)" target="_blank" rel="noopener" class="inline-flex items-center gap-1 text-xs text-primary">打开目录<ExternalLink class="size-3" /></a></div>
            <article v-for="artifact in visibleArtifacts" :key="artifact.id" class="overflow-hidden rounded-lg border border-border/70 bg-card">
              <img v-if="!artifact.removed && artifact.kind === 'image' && artifact.preview_url && !previewErrors.has(artifact.id)" :src="artifact.preview_url" :alt="artifact.relative_path" loading="lazy" class="max-h-60 w-full bg-muted/50 object-contain" @error="markPreviewError(artifact)">
              <video v-else-if="!artifact.removed && artifact.kind === 'video' && artifact.preview_url && !previewErrors.has(artifact.id)" :src="artifact.preview_url" controls preload="none" class="max-h-60 w-full bg-black" @error="markPreviewError(artifact)" />
              <div class="space-y-1 p-3"><p v-if="!artifact.removed && previewErrors.has(artifact.id)" class="text-xs text-muted-foreground">预览加载失败，可下载查看</p><p class="flex items-start gap-2 text-xs"><Image v-if="artifact.kind === 'image'" class="size-3.5 shrink-0" /><Video v-else-if="artifact.kind === 'video'" class="size-3.5 shrink-0" /><File v-else class="size-3.5 shrink-0" /><span class="break-all">{{ artifact.relative_path }}</span></p><div class="flex items-center justify-between text-xs"><span class="text-muted-foreground">{{ artifact.size !== undefined ? size(artifact.size) : '' }}</span><span v-if="artifact.removed" class="text-muted-foreground">已删除</span><a v-else :href="artifact.download_url" download class="inline-flex items-center gap-1 text-primary underline"><Download class="size-3" />下载</a></div></div>
            </article>
            <Button v-if="selected.artifacts.length > 4" variant="ghost" size="sm" class="w-full text-xs" @click="showAllArtifacts = !showAllArtifacts">{{ showAllArtifacts ? '收起产物' : `查看全部 ${selected.artifacts.length} 个产物` }}</Button>
          </section>
        </template>
        <section v-if="tasks.length" aria-label="近期任务" class="space-y-2 border-t border-border/60 pt-3">
          <div class="flex items-center justify-between px-1"><h3 class="text-xs font-medium text-muted-foreground">近期任务</h3><Button v-if="!followCurrent" variant="ghost" size="sm" class="h-6 text-xs" @click="selectCurrent">返回当前任务</Button></div>
          <button v-for="task in tasks" :key="task.id" type="button" class="flex w-full items-start justify-between gap-2 rounded-lg border p-3 text-left text-sm outline-none transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring" :class="task.id === selectedId ? 'border-primary/40 bg-muted/40' : 'border-border/70 bg-card'" :aria-pressed="task.id === selectedId" @click="choose(task)"><span class="min-w-0"><span class="block break-words font-medium">{{ task.title }}</span><span class="mt-1 block text-xs text-muted-foreground">{{ formatTime(task.started_at) }} · {{ task.steps_truncated ? '最近 ' : '' }}{{ task.steps.length }} 步</span></span><span class="shrink-0 text-xs" :class="task.state === 'failed' ? 'text-destructive' : 'text-muted-foreground'">{{ task.source === 'mcp' ? `记录：${taskState[task.state]}` : taskState[task.state] }}</span></button>
        </section>
      </div>
    </ScrollArea>
  </section>
</template>
