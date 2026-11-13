<script setup lang="ts">
import type { Approval, ApprovalDecision, AuditStep, AuditTask } from '@/api/safety'
import { onMounted, onUnmounted, ref } from 'vue'
import { decideApproval, fetchApprovals, fetchAuditTask, fetchAuditTasks, fetchSafetyPolicy, saveSafetyPolicy } from '@/api/safety'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const preset = ref<number>(2)
const approvals = ref<Approval[]>([])
const tasks = ref<AuditTask[]>([])
const selected = ref<AuditTask | null>(null)
const steps = ref<AuditStep[]>([])
const detailOpen = ref(false)
const error = ref('')
const feedback = ref('')
const busy = ref(false)
let active = true
let polling = false
let timer: ReturnType<typeof setInterval> | undefined
let detailSeq = 0

function report(cause: unknown) {
  console.error('[safety] request failed', cause)
  error.value = cause instanceof Error ? cause.message : '请求失败'
}
async function refresh(initial = false) {
  if (polling) return
  polling = true
  try {
    const [pending, history] = await Promise.all([fetchApprovals(), fetchAuditTasks()])
    if (!active) return
    approvals.value = pending.items === null ? [] : pending.items
    tasks.value = history.tasks === null ? [] : history.tasks
    if (initial) {
      const policy = await fetchSafetyPolicy()
      if (active) preset.value = policy.preset_id
    }
    if (active) error.value = ''
  }
  catch (cause) { if (active) report(cause) }
  finally { polling = false }
}
async function save() {
  busy.value = true
  feedback.value = ''
  try {
    await saveSafetyPolicy(preset.value)
    const actual = await fetchSafetyPolicy()
    if (!active) return
    if (actual.preset_id !== preset.value) throw new Error('保存结果不一致')
    feedback.value = '已保存'
    error.value = ''
  }
  catch (cause) { if (active) report(cause) }
  finally { if (active) busy.value = false }
}
async function decide(item: Approval, decision: ApprovalDecision) {
  busy.value = true
  try { await decideApproval(item.id, decision); await refresh() }
  catch (cause) { if (active) report(cause) }
  finally { if (active) busy.value = false }
}
async function showDetail(task: AuditTask) {
  const seq = ++detailSeq
  try {
    const data = await fetchAuditTask(task.id)
    if (!active || seq !== detailSeq) return
    selected.value = data.task
    steps.value = data.steps === null ? [] : data.steps
    detailOpen.value = true
  }
  catch (cause) { if (active && seq === detailSeq) report(cause) }
}
onMounted(() => { void refresh(true); timer = setInterval(() => { void refresh() }, 5000) })
onUnmounted(() => { active = false; detailSeq++; if (timer) clearInterval(timer) })
</script>

<template>
  <section class="rounded-3xl border border-border/60 bg-card/80 p-6 shadow-sm" aria-label="安全审核">
    <div class="mb-5 flex items-center justify-between gap-3">
      <h2 class="text-base font-semibold tracking-tight text-foreground sm:text-lg">
        安全审核
      </h2>
      <Button variant="outline" size="sm" @click="refresh(true)">刷新</Button>
    </div>
    <div class="flex flex-wrap items-center gap-3">
      <label for="safety-preset" class="text-sm">审核策略</label>
      <Select
        :model-value="String(preset)"
        :disabled="busy"
        @update:model-value="value => { preset = Number(value) }"
      >
        <SelectTrigger id="safety-preset" class="w-44" aria-label="审核策略">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="2">支付操作审核</SelectItem>
          <SelectItem value="1">敏感操作审核</SelectItem>
          <SelectItem value="3">每步审核</SelectItem>
          <SelectItem value="0">仅自定义规则</SelectItem>
        </SelectContent>
      </Select>
      <Button :disabled="busy" @click="save">保存</Button>
      <span role="status" class="text-sm text-muted-foreground">{{ feedback }}</span>
    </div>
    <p v-if="error" role="alert" class="mt-3 text-sm text-destructive">{{ error }}</p>
    <div class="mt-6 border-t pt-5">
      <h3 class="mb-3 font-medium">待审核 <span class="text-muted-foreground">{{ approvals.length }}</span></h3>
      <p v-if="!approvals.length" class="text-sm text-muted-foreground">暂无待审核操作</p>
      <article v-for="item in approvals" :key="item.id" class="mb-3 rounded-2xl border border-border/60 bg-background/60 p-4">
        <div class="font-medium">{{ item.title }}</div>
        <p v-if="item.intent" class="mt-1 text-sm">{{ item.intent }}</p>
        <p v-if="item.detail" class="mt-1 text-sm text-muted-foreground">{{ item.detail }}</p>
        <p class="mt-2 text-xs text-muted-foreground">{{ item.binding.device }} · {{ item.binding.tool }} · {{ new Date(item.deadline).toLocaleTimeString() }} 到期</p>
        <div class="mt-3 flex gap-2">
          <Button size="sm" :disabled="busy" @click="decide(item, 'allow_once')">允许一次</Button>
          <Button size="sm" variant="destructive" :disabled="busy" @click="decide(item, 'deny')">拒绝</Button>
          <Button size="sm" variant="outline" :disabled="busy" @click="decide(item, 'take_over')">人工接管</Button>
        </div>
      </article>
    </div>
    <div class="mt-6 border-t pt-5">
      <h3 class="mb-3 font-medium">任务审计</h3>
      <p v-if="!tasks.length" class="text-sm text-muted-foreground">暂无任务记录</p>
      <button v-for="task in tasks" :key="task.id" class="mb-2 flex w-full items-center justify-between gap-3 rounded-2xl border p-4 text-left hover:bg-muted/50" @click="showDetail(task)">
        <span><span class="block font-medium">{{ task.title }}</span><span class="text-xs text-muted-foreground">{{ task.device_id }} · {{ new Date(task.started_at).toLocaleString() }}</span></span>
        <span class="text-sm">{{ task.state }} · {{ task.step_count }} 步 · {{ task.approval_count }} 次审核</span>
      </button>
    </div>
    <Dialog v-model:open="detailOpen">
      <DialogContent class="max-h-[80vh] overflow-y-auto rounded-3xl sm:max-w-2xl">
        <DialogHeader><DialogTitle>{{ selected?.title }}</DialogTitle></DialogHeader>
        <template v-if="selected">
          <p v-if="selected.summary" class="text-sm">{{ selected.summary }}</p>
          <a :href="`/api/safety/history/tasks/${encodeURIComponent(selected.id)}/export`" target="_blank" rel="noopener" class="text-sm text-blue-600 underline">导出报告</a>
          <p v-if="!steps.length" class="text-sm text-muted-foreground">暂无步骤记录</p>
          <article v-for="step in steps" :key="step.index" class="rounded-2xl border border-border/60 bg-background/60 p-4">
            <div class="flex justify-between text-sm"><span>{{ step.index }} · {{ step.tool }}</span><span>{{ step.status }} · {{ step.elapsed_ms }} ms</span></div>
            <p v-if="step.intent" class="mt-2 text-sm">{{ step.intent }}</p>
            <pre class="mt-2 whitespace-pre-wrap break-all text-xs text-muted-foreground">{{ step.args }}</pre>
          </article>
        </template>
      </DialogContent>
    </Dialog>
  </section>
</template>
