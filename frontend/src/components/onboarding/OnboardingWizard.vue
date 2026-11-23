<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import {
  ArrowRight,
  Check,
  Download,
  Loader2,
  Plus,
  Smartphone,
  Terminal,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { useDependencies, type DependencyId, type ManagedTool } from '@/composables/useDependencies'
import { GLASS_CARD_SURFACE } from '@/lib/surfaces'

const props = defineProps<{ open: boolean; deviceCount: number }>()
const emit = defineEmits<{
  'update:open': [value: boolean]
  'add-device': []
  'completed': []
}>()

const w = useDependencies()
const step = ref(0)
const licenseAccepted = ref(false)
const applying = ref(false)
const installing = ref<Set<DependencyId>>(new Set())

const steps = [
  { title: '安装 ADB', subtitle: '连接 Android 设备' },
  { title: '安装 Python', subtitle: '运行自动化能力' },
  { title: '添加设备', subtitle: '开始使用 Farm' },
]

function tool(id: DependencyId): ManagedTool | undefined {
  return w.status.value?.managed.find(item => item.id === id)
}

function installed(id: DependencyId) {
  return Boolean(tool(id)?.effective.installed)
}

function taskBusy(value?: ManagedTool) {
  return Boolean(value && ['downloading', 'verifying', 'publishing', 'removing'].includes(value.task.stage))
}

function taskLabel(value?: ManagedTool) {
  if (!value) return ''
  return ({
    downloading: '正在下载',
    verifying: '正在校验',
    publishing: '正在安装',
    removing: '正在移除',
    failed: '安装失败',
    cancelled: '已取消',
    ready: '安装完成',
  } as Record<string, string>)[value.task.stage] ?? ''
}

function percent(value?: ManagedTool) {
  if (!value || value.task.total <= 0) return 0
  return Math.min(100, Math.floor(value.task.received / value.task.total * 100))
}

const stepDone = computed(() => [installed('adb'), installed('python'), props.deviceCount > 0])
const allDone = computed(() => stepDone.value.every(Boolean))
const currentDone = computed(() => stepDone.value[step.value])
const canInstallAdb = computed(() => licenseAccepted.value || Boolean(tool('adb')?.installed))

watch(() => props.open, (open) => {
  if (!open) {
    w.stopPolling()
    return
  }
  step.value = 0
  licenseAccepted.value = false
  void w.load().then(() => {
    if (props.open) w.startPolling()
  })
}, { immediate: true })

watch(() => w.status.value?.managed.map(item => `${item.id}:${item.task.stage}:${item.effective.installed}`).join('|'), () => {
  void applyReadyInstalls()
})

onUnmounted(() => w.stopPolling())

async function install(id: DependencyId) {
  if (id === 'adb' && !licenseAccepted.value) return
  installing.value = new Set([...installing.value, id])
  await w.action(id, '/api/dependencies/install', 'POST', {
    id,
    accept_license: id === 'adb' && licenseAccepted.value,
  })
  if (w.error.value) installing.value = new Set([...installing.value].filter(item => item !== id))
}

async function applyReadyInstalls() {
  if (!props.open || applying.value) return
  const ready = [...installing.value].filter((id) => {
    const item = tool(id)
    return item?.installed && item.task.stage === 'ready' && !item.effective.installed
  })
  if (!ready.length) return
  applying.value = true
  try {
    for (const id of ready) {
      const next = structuredClone(w.draft.value)
      next[id] = { source: 'builtin', path: '' }
      w.draft.value = next
      await w.save(true)
      if (!w.error.value) installing.value = new Set([...installing.value].filter(item => item !== id))
    }
  }
  finally {
    applying.value = false
  }
}

async function enableBuiltin(id: DependencyId) {
  if (applying.value) return
  applying.value = true
  const previous = structuredClone(w.draft.value[id])
  try {
    const next = structuredClone(w.draft.value)
    next[id] = { source: 'builtin', path: '' }
    w.draft.value = next
    await w.save(true)
    if (w.error.value) w.draft.value[id] = previous
  }
  finally {
    applying.value = false
  }
}

function next() {
  if (step.value < steps.length - 1) step.value += 1
  else if (allDone.value) emit('completed')
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        class="fixed inset-0 z-[70] flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-sm"
        role="dialog"
        aria-modal="true"
        aria-labelledby="onboarding-title"
      >
        <div :class="GLASS_CARD_SURFACE" class="flex max-h-[calc(100dvh-2rem)] w-full max-w-2xl flex-col overflow-hidden">
          <header class="shrink-0 border-b border-slate-200/70 px-6 pb-5 pt-6 dark:border-white/10">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="text-[11px] font-semibold uppercase tracking-[0.18em] text-blue-600 dark:text-blue-400">首次使用</p>
                <h2 id="onboarding-title" class="mt-1 text-xl font-bold text-slate-900 dark:text-white">初始化 Farm</h2>
                <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">完成以下三步，即可连接和控制 Android 设备。</p>
              </div>
              <button
                type="button"
                class="shrink-0 rounded-full px-2 py-1 text-xs text-slate-400 transition-colors hover:bg-black/5 hover:text-slate-700 dark:hover:bg-white/10 dark:hover:text-white"
                @click="emit('completed')"
              >
                稍后设置
              </button>
            </div>

            <ol class="mt-5 grid grid-cols-3 gap-2">
              <li v-for="(item, index) in steps" :key="item.title">
                <button
                  type="button"
                  class="flex w-full items-center gap-2 rounded-2xl border px-3 py-2 text-left transition-colors"
                  :class="step === index
                    ? 'border-blue-500/50 bg-blue-50/80 dark:border-blue-400/40 dark:bg-blue-500/10'
                    : 'border-slate-200/70 bg-white/50 hover:bg-white/80 dark:border-white/10 dark:bg-white/[0.03] dark:hover:bg-white/[0.06]'"
                  @click="step = index"
                >
                  <span
                    class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-[11px] font-bold"
                    :class="stepDone[index]
                      ? 'bg-emerald-500 text-white'
                      : step === index
                        ? 'bg-blue-600 text-white'
                        : 'bg-slate-200 text-slate-500 dark:bg-white/10 dark:text-slate-300'"
                  >
                    <Check v-if="stepDone[index]" class="h-3.5 w-3.5" />
                    <template v-else>{{ index + 1 }}</template>
                  </span>
                  <span class="min-w-0">
                    <span class="block truncate text-xs font-semibold text-slate-800 dark:text-slate-100">{{ item.title }}</span>
                    <span class="block truncate text-[10px] text-slate-400">{{ item.subtitle }}</span>
                  </span>
                </button>
              </li>
            </ol>
          </header>

          <div class="min-h-[280px] flex-1 overflow-y-auto px-6 py-6">
            <div v-if="!w.status.value && !w.error.value" class="flex h-full min-h-[220px] items-center justify-center gap-2 text-sm text-slate-500">
              <Loader2 class="h-4 w-4 animate-spin" />
              正在检测运行环境…
            </div>
            <div v-else-if="!w.status.value" class="flex h-full min-h-[220px] flex-col items-center justify-center text-center">
              <p class="text-sm text-red-600 dark:text-red-400">{{ w.error.value }}</p>
              <Button variant="outline" size="sm" class="mt-3" @click="w.load()">重新检测</Button>
            </div>

            <template v-else-if="step === 0">
              <div class="flex items-start gap-4">
                <div class="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-blue-600/10 text-blue-600 dark:bg-blue-400/10 dark:text-blue-400">
                  <Terminal class="h-6 w-6" />
                </div>
                <div class="min-w-0 flex-1">
                  <h3 class="text-base font-semibold text-slate-900 dark:text-white">安装 ADB</h3>
                  <p class="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">ADB 是连接和控制 Android 设备的基础工具，用于发现设备、投屏和自动化操作。</p>

                  <div v-if="installed('adb')" class="mt-5 rounded-2xl border border-emerald-500/25 bg-emerald-50/80 p-4 dark:bg-emerald-500/10">
                    <p class="flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-300"><Check class="h-4 w-4" />ADB 已就绪</p>
                    <p class="mt-1 break-all text-xs text-emerald-700/80 dark:text-emerald-300/80">{{ tool('adb')?.effective.version }}</p>
                    <p class="mt-1 break-all text-[11px] text-emerald-700/60 dark:text-emerald-300/60">{{ tool('adb')?.effective.path }}</p>
                  </div>

                  <div v-else-if="taskBusy(tool('adb'))" class="mt-5 rounded-2xl border border-blue-500/20 bg-blue-50/70 p-4 dark:bg-blue-500/10">
                    <p class="flex items-center gap-2 text-sm font-medium text-blue-700 dark:text-blue-300"><Loader2 class="h-4 w-4 animate-spin" />{{ taskLabel(tool('adb')) }}</p>
                    <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-blue-200/70 dark:bg-white/10">
                      <div class="h-full rounded-full bg-blue-600 transition-all" :class="{ 'w-1/3 animate-pulse': (tool('adb')?.task.total ?? 0) <= 0 }" :style="(tool('adb')?.task.total ?? 0) > 0 ? { width: `${percent(tool('adb'))}%` } : undefined" />
                    </div>
                    <p v-if="(tool('adb')?.task.received ?? 0) > 0" class="mt-2 text-[11px] text-blue-700/70 dark:text-blue-300/70">{{ ((tool('adb')?.task.received ?? 0) / 1024 / 1024).toFixed(1) }} MiB<span v-if="(tool('adb')?.task.total ?? 0) > 0"> · {{ percent(tool('adb')) }}%</span></p>
                  </div>

                  <div v-else class="mt-5 space-y-3">
                    <div v-if="tool('adb')?.installed" class="rounded-2xl border border-amber-500/25 bg-amber-50/80 p-4 dark:bg-amber-500/10">
                      <p class="text-sm font-medium text-amber-700 dark:text-amber-300">内置 ADB 已下载</p>
                      <p class="mt-1 text-xs text-amber-700/70 dark:text-amber-300/70">启用后才能使用内置版本。</p>
                      <Button size="sm" class="mt-3 bg-blue-600 text-white hover:bg-blue-700" :disabled="applying" @click="enableBuiltin('adb')">启用内置 ADB</Button>
                    </div>
                    <template v-else>
                      <label class="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
                        <Checkbox :model-value="licenseAccepted" @update:model-value="value => licenseAccepted = value === true" />
                        我已阅读并同意
                        <a href="https://developer.android.com/studio/terms" target="_blank" rel="noopener" class="text-blue-600 underline underline-offset-2 dark:text-blue-400">Android SDK 许可协议</a>
                      </label>
                      <Button class="bg-blue-600 text-white hover:bg-blue-700" :disabled="!canInstallAdb" @click="install('adb')">
                        <Download class="h-4 w-4" />
                        {{ tool('adb')?.task.stage === 'failed' ? '重新安装 ADB' : '安装 ADB' }}
                      </Button>
                    </template>
                  </div>
                  <p v-if="w.error.value" class="mt-3 text-xs text-red-600 dark:text-red-400">{{ w.error.value }}</p>
                  <p v-else-if="tool('adb')?.task.error" class="mt-3 text-xs text-red-600 dark:text-red-400">{{ tool('adb')?.task.error }}</p>
                </div>
              </div>
            </template>

            <template v-else-if="step === 1">
              <div class="flex items-start gap-4">
                <div class="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-cyan-600/10 text-cyan-600 dark:bg-cyan-400/10 dark:text-cyan-400">
                  <Terminal class="h-6 w-6" />
                </div>
                <div class="min-w-0 flex-1">
                  <h3 class="text-base font-semibold text-slate-900 dark:text-white">安装 Python</h3>
                  <p class="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">Python 用于模板匹配、OCR 等自动化能力，安装后由 Farm 自动管理运行环境。</p>

                  <div v-if="installed('python')" class="mt-5 rounded-2xl border border-emerald-500/25 bg-emerald-50/80 p-4 dark:bg-emerald-500/10">
                    <p class="flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-300"><Check class="h-4 w-4" />Python 已就绪</p>
                    <p class="mt-1 break-all text-xs text-emerald-700/80 dark:text-emerald-300/80">{{ tool('python')?.effective.version }}</p>
                    <p class="mt-1 break-all text-[11px] text-emerald-700/60 dark:text-emerald-300/60">{{ tool('python')?.effective.path }}</p>
                  </div>

                  <div v-else-if="taskBusy(tool('python'))" class="mt-5 rounded-2xl border border-blue-500/20 bg-blue-50/70 p-4 dark:bg-blue-500/10">
                    <p class="flex items-center gap-2 text-sm font-medium text-blue-700 dark:text-blue-300"><Loader2 class="h-4 w-4 animate-spin" />{{ taskLabel(tool('python')) }}</p>
                    <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-blue-200/70 dark:bg-white/10">
                      <div class="h-full rounded-full bg-blue-600 transition-all" :class="{ 'w-1/3 animate-pulse': (tool('python')?.task.total ?? 0) <= 0 }" :style="(tool('python')?.task.total ?? 0) > 0 ? { width: `${percent(tool('python'))}%` } : undefined" />
                    </div>
                    <p v-if="(tool('python')?.task.received ?? 0) > 0" class="mt-2 text-[11px] text-blue-700/70 dark:text-blue-300/70">{{ ((tool('python')?.task.received ?? 0) / 1024 / 1024).toFixed(1) }} MiB<span v-if="(tool('python')?.task.total ?? 0) > 0"> · {{ percent(tool('python')) }}%</span></p>
                  </div>

                  <div v-else class="mt-5">
                    <div v-if="tool('python')?.installed" class="rounded-2xl border border-amber-500/25 bg-amber-50/80 p-4 dark:bg-amber-500/10">
                      <p class="text-sm font-medium text-amber-700 dark:text-amber-300">内置 Python 已下载</p>
                      <p class="mt-1 text-xs text-amber-700/70 dark:text-amber-300/70">启用后才能使用内置版本。</p>
                      <Button size="sm" class="mt-3 bg-blue-600 text-white hover:bg-blue-700" :disabled="applying" @click="enableBuiltin('python')">启用内置 Python</Button>
                    </div>
                    <Button v-else class="bg-blue-600 text-white hover:bg-blue-700" @click="install('python')">
                      <Download class="h-4 w-4" />
                      {{ tool('python')?.task.stage === 'failed' ? '重新安装 Python' : '安装 Python' }}
                    </Button>
                  </div>
                  <p v-if="w.error.value" class="mt-3 text-xs text-red-600 dark:text-red-400">{{ w.error.value }}</p>
                  <p v-else-if="tool('python')?.task.error" class="mt-3 text-xs text-red-600 dark:text-red-400">{{ tool('python')?.task.error }}</p>
                </div>
              </div>
            </template>

            <template v-else>
              <div class="flex items-start gap-4">
                <div class="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-violet-600/10 text-violet-600 dark:bg-violet-400/10 dark:text-violet-400">
                  <Smartphone class="h-6 w-6" />
                </div>
                <div class="min-w-0 flex-1">
                  <h3 class="text-base font-semibold text-slate-900 dark:text-white">添加设备</h3>
                  <p class="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">用 USB 数据线连接手机并开启 USB 调试，或通过 IP 地址和端口添加无线设备。</p>

                  <div v-if="deviceCount > 0" class="mt-5 rounded-2xl border border-emerald-500/25 bg-emerald-50/80 p-4 dark:bg-emerald-500/10">
                    <p class="flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-300"><Check class="h-4 w-4" />已添加 {{ deviceCount }} 台设备</p>
                    <p class="mt-1 text-xs text-emerald-700/70 dark:text-emerald-300/70">可以继续添加，或完成初始化进入设备列表。</p>
                  </div>

                  <Button class="mt-5 bg-blue-600 text-white hover:bg-blue-700" @click="emit('add-device')">
                    <Plus class="h-4 w-4" />
                    {{ deviceCount > 0 ? '继续添加设备' : '添加设备' }}
                  </Button>
                  <p class="mt-3 text-[11px] leading-relaxed text-slate-400">如果设备没有出现，请确认手机上已授权 USB 调试，并重新插拔数据线。</p>
                </div>
              </div>
            </template>
          </div>

          <footer class="flex shrink-0 items-center justify-between gap-3 border-t border-slate-200/70 px-6 py-4 dark:border-white/10">
            <Button variant="ghost" :disabled="step === 0" @click="step -= 1">上一步</Button>
            <div class="flex items-center gap-2">
              <span v-if="!allDone" class="text-[11px] text-slate-400">完成当前步骤后继续</span>
              <Button
                class="bg-blue-600 text-white hover:bg-blue-700"
                :disabled="!currentDone"
                @click="next"
              >
                {{ step === steps.length - 1 ? '完成' : '下一步' }}
                <ArrowRight v-if="step < steps.length - 1" class="h-4 w-4" />
                <Check v-else class="h-4 w-4" />
              </Button>
            </div>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
