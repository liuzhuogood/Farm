<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  BookOpen,
  Check,
  Copy,
  Download,
  ExternalLink,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { downloadSkillZip, fetchContractContent, type AiTool } from '@/api/aiIntegrations'

const props = defineProps<{
  contractUrl?: string
  currentTool?: AiTool | null
  isInstalled?: boolean
}>()

const emit = defineEmits<{
  (e: 'install'): void
}>()

const activeTab = ref('workflow')
const contractText = ref('')
const loadingDoc = ref(false)
const docError = ref('')

const isDownloading = ref(false)
const copyKey = ref('')
let copyTimer: ReturnType<typeof setTimeout> | null = null

const curlExample1 = `# 1. 查看设备
curl -s http://127.0.0.1:1989/api/automation/devices

# 2. 开始任务
TASK_JSON=$(curl -s -X POST 'http://127.0.0.1:1989/api/automation/tasks?serial=10.0.0.30:5555&title=检查设置')
TASK_ID=$(echo $TASK_JSON | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

# 3. 观察当前屏幕
curl -s 'http://127.0.0.1:1989/api/automation/observe?serial=10.0.0.30:5555&format=text'

# 4. 执行操作批次
curl -s --data-binary @- "http://127.0.0.1:1989/api/automation/act?serial=10.0.0.30:5555&title=检查设置&task_id=\${TASK_ID}" << 'EOF'
tap @540,960
sleep 500
wait 10000 WLAN
check WLAN
EOF

# 5. 结束任务
curl -s -X POST -H 'Content-Type: application/json' \\
  -d '{"outcome":"done","summary":"成功验证 WLAN 设置"}' \\
  "http://127.0.0.1:1989/api/automation/tasks/\${TASK_ID}/finish?serial=10.0.0.30:5555"`

async function handleCopy(key: string, text: string) {
  if (copyTimer) {
    clearTimeout(copyTimer)
    copyTimer = null
  }
  if (!navigator?.clipboard?.writeText)
    return
  try {
    await navigator.clipboard.writeText(text)
    copyKey.value = key
    copyTimer = setTimeout(() => {
      if (copyKey.value === key)
        copyKey.value = ''
    }, 2000)
  }
  catch {
    // ignore
  }
}

async function loadContract() {
  loadingDoc.value = true
  docError.value = ''
  try {
    contractText.value = await fetchContractContent()
  }
  catch (err) {
    docError.value = err instanceof Error ? err.message : '加载技能文档失败'
  }
  finally {
    loadingDoc.value = false
  }
}

async function handleDownloadZip() {
  if (isDownloading.value)
    return
  isDownloading.value = true
  try {
    await downloadSkillZip('farm-device-base', 'farm-device-base.zip')
  }
  catch (err) {
    console.error('下载技能包失败', err)
  }
  finally {
    isDownloading.value = false
  }
}

const displayContractUrl = computed(() => props.contractUrl || '/fram.skill')

onMounted(() => {
  loadContract()
})

onUnmounted(() => {
  if (copyTimer)
    clearTimeout(copyTimer)
})
</script>

<template>
  <section class="rounded-3xl border border-border/60 bg-card/80 p-6 shadow-sm space-y-5" aria-label="自动化技能契约与说明">
    <!-- 头部：标题、契约地址与合并安装按钮 -->
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <div class="flex flex-wrap items-center gap-2.5">
          <div class="flex h-8 w-8 items-center justify-center rounded-xl bg-blue-500/10 text-blue-600 dark:bg-blue-500/20 dark:text-blue-400">
            <BookOpen class="h-4 w-4" />
          </div>
          <h2 class="text-base font-semibold tracking-tight text-foreground sm:text-lg">
            自动化技能文档 · 接口契约
          </h2>
          <Badge variant="outline" class="font-mono text-xs text-blue-600 dark:text-blue-400 border-blue-500/30">
            /fram.skill
          </Badge>
          <Badge v-if="props.isInstalled" variant="outline" class="border-emerald-500/40 text-emerald-600 dark:text-emerald-400 font-normal text-xs">
            已安装至 {{ props.currentTool?.name || '当前助手' }}
          </Badge>
        </div>
        <p class="mt-1.5 text-xs text-muted-foreground leading-relaxed">
          以技能文档 / 技能接口页面为唯一权威来源的统一设备自动化技能。支持纯文本设备发现、完整任务生命周期、统一感知与多行文本动作协议。
        </p>
      </div>

      <!-- 统一操作区：安装、下载与查看原始页 -->
      <div class="flex flex-wrap items-center gap-2">
        <Button size="sm" class="h-8 gap-1.5" @click="emit('install')">
          <Download class="h-3.5 w-3.5" />
          <span>{{ props.isInstalled ? '更新/重新安装技能' : '安装基础技能' }}</span>
        </Button>
        <Button variant="outline" size="sm" class="h-8 gap-1.5" :disabled="isDownloading" @click="handleDownloadZip">
          <Download class="h-3.5 w-3.5" />
          <span>直接下载 (.zip)</span>
        </Button>
        <Button variant="ghost" size="sm" class="h-8 gap-1" as-child>
          <a :href="displayContractUrl" target="_blank" rel="noopener noreferrer">
            <span>独立网页</span>
            <ExternalLink class="h-3.5 w-3.5" />
          </a>
        </Button>
      </div>
    </div>

    <!-- 选项卡切换 -->
    <Tabs v-model="activeTab" class="w-full">
      <TabsList class="grid grid-cols-4 max-w-md h-8 p-0.5">
        <TabsTrigger value="workflow" class="text-xs">核心流程</TabsTrigger>
        <TabsTrigger value="actions" class="text-xs">动作速查</TabsTrigger>
        <TabsTrigger value="curl" class="text-xs">curl 示例</TabsTrigger>
        <TabsTrigger value="raw" class="text-xs">文档全文</TabsTrigger>
      </TabsList>

      <!-- 选项卡 1：核心流程 -->
      <TabsContent value="workflow" class="mt-4 space-y-3">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
          <div class="rounded-2xl border border-border/60 bg-background/50 p-3.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[11px] font-bold text-blue-700 dark:bg-blue-950 dark:text-blue-300">1</span>
              <h4 class="text-xs font-semibold text-foreground">选择设备</h4>
            </div>
            <code class="block font-mono text-[11px] bg-muted/60 px-2 py-1 rounded text-foreground">
              GET /api/automation/devices
            </code>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              一行一台在线设备（标识名、serial、在线状态）。从返回值确定真实 serial。
            </p>
          </div>

          <div class="rounded-2xl border border-border/60 bg-background/50 p-3.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[11px] font-bold text-blue-700 dark:bg-blue-950 dark:text-blue-300">2</span>
              <h4 class="text-xs font-semibold text-foreground">开始任务 (Task)</h4>
            </div>
            <code class="block font-mono text-[11px] bg-muted/60 px-2 py-1 rounded text-foreground">
              POST /api/automation/tasks?serial=...&title=...
            </code>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              由 AI 命名任务，获取 task_id 与产物目录，全程关联所有步骤。
            </p>
          </div>

          <div class="rounded-2xl border border-border/60 bg-background/50 p-3.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[11px] font-bold text-blue-700 dark:bg-blue-950 dark:text-blue-300">3</span>
              <h4 class="text-xs font-semibold text-foreground">统一感知 (Observe)</h4>
            </div>
            <code class="block font-mono text-[11px] bg-muted/60 px-2 py-1 rounded text-foreground">
              GET /api/automation/observe?serial=...&format=text
            </code>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              返回屏幕像素尺寸、应用名、带像素坐标 @X,Y 的控件与 OCR 文字。
            </p>
          </div>

          <div class="rounded-2xl border border-border/60 bg-background/50 p-3.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[11px] font-bold text-blue-700 dark:bg-blue-950 dark:text-blue-300">4</span>
              <h4 class="text-xs font-semibold text-foreground">多行文本动作 (Act)</h4>
            </div>
            <code class="block font-mono text-[11px] bg-muted/60 px-2 py-1 rounded text-foreground">
              POST /api/automation/act?serial=...&task_id=...
            </code>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              正文多行纯文本指令（tap、swipe、sendkey、wait、check等），按序批量执行。
            </p>
          </div>

          <div class="rounded-2xl border border-border/60 bg-background/50 p-3.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[11px] font-bold text-blue-700 dark:bg-blue-950 dark:text-blue-300">5</span>
              <h4 class="text-xs font-semibold text-foreground">调用日志与产物</h4>
            </div>
            <code class="block font-mono text-[11px] bg-muted/60 px-2 py-1 rounded text-foreground">
              GET /api/automation/logs?serial=...
            </code>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              逐行返回带步骤序号与耗时的文本日志；通过 /artifacts 下载截图录屏。
            </p>
          </div>

          <div class="rounded-2xl border border-border/60 bg-background/50 p-3.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <span class="flex h-5 w-5 items-center justify-center rounded-full bg-blue-100 text-[11px] font-bold text-blue-700 dark:bg-blue-950 dark:text-blue-300">6</span>
              <h4 class="text-xs font-semibold text-foreground">结束任务 (Finish)</h4>
            </div>
            <code class="block font-mono text-[11px] bg-muted/60 px-2 py-1 rounded text-foreground">
              POST /api/automation/tasks/{id}/finish
            </code>
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              提交 outcome: done/failed/stopped 及最终业务核对摘要，闭环任务生命周期。
            </p>
          </div>
        </div>
      </TabsContent>

      <!-- 选项卡 2：动作速查 -->
      <TabsContent value="actions" class="mt-4">
        <div class="rounded-2xl border border-border/60 overflow-hidden text-xs">
          <div class="bg-muted/40 px-3.5 py-2 font-semibold text-foreground border-b border-border/60">
            多行纯文本动作指令（/api/automation/act 请求正文，每行一条）
          </div>
          <div class="divide-y divide-border/60 font-mono">
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">tap @X,Y</span>
              <span class="text-muted-foreground font-sans">点击屏幕像素坐标，例如：tap @540,960</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">keyevent &lt;keys&gt;</span>
              <span class="text-muted-foreground font-sans">系统按键（支持逗号或竖线拼接）：home｜back,app_switch,enter</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">sendkey &lt;text&gt;</span>
              <span class="text-muted-foreground font-sans">向当前焦点输入框注入文本：sendkey 你好 Farm</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">set-copy &lt;text&gt;</span>
              <span class="text-muted-foreground font-sans">写入手机系统剪贴板（不触发粘贴）：set-copy 内容</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">copy / paste / clear</span>
              <span class="text-muted-foreground font-sans">无参数：复制选中文字 / 粘贴剪贴板 / 清空当前输入框</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">swipe @X1,Y1 @X2,Y2 [ms]</span>
              <span class="text-muted-foreground font-sans">手势滑动：swipe @540,1800 @540,500 300</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">check &lt;text&gt;</span>
              <span class="text-muted-foreground font-sans">页面文字存在性检查，未命中则中断：check 设置</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">wait &lt;ms&gt; &lt;text&gt;</span>
              <span class="text-muted-foreground font-sans">等待目标文字出现：wait 10000 WLAN（超时中断）</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">sleep &lt;ms&gt;</span>
              <span class="text-muted-foreground font-sans">固定毫秒延时：sleep 500</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">adb &lt;shell ...&gt;</span>
              <span class="text-muted-foreground font-sans">安全 ADB 子命令：adb shell getprop ro.product.model</span>
            </div>
            <div class="flex flex-wrap items-center justify-between p-2.5 px-3.5 hover:bg-muted/30">
              <span class="font-bold text-blue-600 dark:text-blue-400">mkdir / screenshot / record / rm</span>
              <span class="text-muted-foreground font-sans">设备产物操作（相对目录位于 /sdcard/Farm/automation/）</span>
            </div>
          </div>
        </div>
      </TabsContent>

      <!-- 选项卡 3：curl 示例 -->
      <TabsContent value="curl" class="mt-4 space-y-3">
        <div class="rounded-2xl border border-border/60 bg-muted/40 p-3.5">
          <div class="flex items-center justify-between mb-2">
            <span class="text-xs font-semibold text-foreground">示例 1：开始任务 → 观察 → 点击并等待 → 结束任务</span>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 px-2 text-[11px] gap-1"
              @click="handleCopy('curl1', curlExample1)"
            >
              <Check v-if="copyKey === 'curl1'" class="h-3 w-3 text-emerald-500" />
              <Copy v-else class="h-3 w-3" />
              <span>{{ copyKey === 'curl1' ? '已复制' : '复制命令' }}</span>
            </Button>
          </div>
          <pre class="overflow-x-auto text-[11px] font-mono leading-relaxed text-foreground/90 bg-background/80 p-3 rounded-xl border border-border/40"># 1. 查看设备
curl -s http://127.0.0.1:1989/api/automation/devices

# 2. 开始任务
TASK_JSON=$(curl -s -X POST 'http://127.0.0.1:1989/api/automation/tasks?serial=10.0.0.30:5555&title=检查设置')
TASK_ID=$(echo $TASK_JSON | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

# 3. 观察当前屏幕
curl -s 'http://127.0.0.1:1989/api/automation/observe?serial=10.0.0.30:5555&format=text'

# 4. 执行操作批次 (多行纯文本输入)
curl -s --data-binary @- "http://127.0.0.1:1989/api/automation/act?serial=10.0.0.30:5555&title=检查设置&task_id=${TASK_ID}" << 'EOF'
tap @540,960
sleep 500
wait 10000 WLAN
check WLAN
EOF

# 5. 结束任务
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"outcome":"done","summary":"成功验证 WLAN 设置"}' \
  "http://127.0.0.1:1989/api/automation/tasks/${TASK_ID}/finish?serial=10.0.0.30:5555"</pre>
        </div>
      </TabsContent>

      <!-- 选项卡 4：文档全文 -->
      <TabsContent value="raw" class="mt-4">
        <div class="relative rounded-2xl border border-border/60 bg-muted/30 p-4">
          <div class="flex items-center justify-between mb-2">
            <span class="text-xs font-semibold text-foreground">服务端渲染 Markdown 契约原文 (/fram.skill)</span>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 px-2 text-[11px] gap-1"
              @click="handleCopy('rawDoc', contractText)"
            >
              <Check v-if="copyKey === 'rawDoc'" class="h-3 w-3 text-emerald-500" />
              <Copy v-else class="h-3 w-3" />
              <span>{{ copyKey === 'rawDoc' ? '已复制' : '复制全文' }}</span>
            </Button>
          </div>
          <div v-if="loadingDoc" class="py-8 text-center text-xs text-muted-foreground">
            正在读取 /fram.skill 契约...
          </div>
          <p v-else-if="docError" class="text-xs text-destructive py-4">{{ docError }}</p>
          <pre v-else class="max-h-[380px] overflow-y-auto whitespace-pre-wrap font-mono text-[11.5px] leading-relaxed text-foreground/90 bg-background/80 p-3 rounded-xl border border-border/40 select-text">{{ contractText }}</pre>
        </div>
      </TabsContent>
    </Tabs>
  </section>
</template>
