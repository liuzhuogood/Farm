<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { invoke, isTauri } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { fetchApprovals, decideApproval, type Approval, type ApprovalDecision } from '@/api/safety'
import { Button } from '@/components/ui/button'
import AppGlyph from '@/components/device/AppGlyph.vue'

const route = useRoute()
const router = useRouter()
const standalone = computed(() => route.path === '/approval')
const native = isTauri()
const items = ref<Approval[]>([])
const error = ref('')
const busy = ref('')
const now = ref(Date.now())
const pending = computed(() => items.value.filter(item => item.state === 0 && Date.parse(item.deadline) > now.value
  && (!standalone.value || item.id === route.query.approval)))
let active = true
let enabled = false
let polling = false
let timer: ReturnType<typeof setInterval> | undefined
let announced = new Set<string>()
async function refresh() {
  if (!active || !enabled || polling || busy.value) return
  polling = true
  try {
    const data = await fetchApprovals()
    if (!active) return
    now.value = Date.now()
    items.value = data.items === null ? [] : data.items
    if (native && !standalone.value) {
      const ids = new Set(pending.value.map(item => item.id))
      for (const id of ids) {
        if (!announced.has(id)) await invoke('open_approval_window', { approvalId: id })
      }
      if (active) announced = ids
    }
    else if (native && standalone.value && pending.value.length === 0) {
      await getCurrentWindow().close()
    }
    if (active) error.value = ''
  }
  catch (cause) {
    console.error('[approval-popup] refresh failed', cause)
    if (active) error.value = cause instanceof Error ? cause.message : '读取审核请求失败'
  }
  finally { polling = false }
}
async function decide(item: Approval, decision: ApprovalDecision) {
  if (busy.value || Date.parse(item.deadline) <= Date.now()) { void refresh(); return }
  busy.value = item.id
  try {
    await decideApproval(item.id, decision)
    if (!active) return
    items.value = items.value.filter(value => value.id !== item.id)
    error.value = ''
  }
  catch (cause) {
    console.error('[approval-popup] decision failed', cause)
    if (active) error.value = cause instanceof Error ? cause.message : '提交审核失败'
  }
  finally { if (active) { busy.value = ''; void refresh() } }
}
onMounted(async () => {
  await router.isReady()
  // Only the main desktop window owns notifications; player windows must not duplicate them.
  enabled = !native || (getCurrentWindow().label === 'main' || getCurrentWindow().label.startsWith('safety-approval-'))
  if (!active || !enabled) return
  void refresh()
  timer = setInterval(() => { now.value = Date.now(); void refresh() }, 2000)
})
onUnmounted(() => { active = false; if (timer) clearInterval(timer) })
</script>

<template>
  <section v-for="(entry, index) in ((!native || standalone) ? pending : [])" :key="entry.id" role="dialog" aria-label="安全审核小窗" :aria-modal="false" :style="!standalone ? { bottom: `${24 + index * 40}px`, right: `${24 + index * 24}px`, zIndex: 100 + index } : undefined" :class="standalone ? 'min-h-screen bg-background' : 'fixed bottom-6 right-6 z-[100] max-h-[80vh] w-[min(420px,calc(100vw-32px))] overflow-y-auto rounded-[28px] border bg-background shadow-xl'">
    <div>
      <header v-if="!native" class="flex items-center justify-between gap-3 px-5 py-4">
        <h2 class="text-sm font-medium text-muted-foreground">安全审核</h2>
      </header>
      <p v-if="error" role="alert" class="mx-5 mb-4 text-sm text-destructive">{{ error }}</p>
      <article v-for="item in [entry]" :key="item.id" :class="!native ? 'border-t border-border/60' : ''">
        <div class="flex gap-3 px-5 pb-5 pt-4">
          <div v-if="item.app_icon" class="flex size-14 shrink-0 items-center justify-center rounded-2xl bg-muted text-foreground" :title="item.app_name" role="img" :aria-label="item.app_name">
            <AppGlyph :icon-type="item.app_icon" size-class="size-10" />
          </div>
          <div class="min-w-0 flex-1">
          <p v-if="item.title !== '审核请求'" class="break-words text-sm leading-relaxed">{{ item.title }}</p>
          <p v-if="item.detail" class="whitespace-pre-wrap break-words text-sm leading-relaxed">{{ item.detail }}</p>
          <div class="mt-4 flex min-w-0 items-center justify-between gap-3 text-xs text-muted-foreground">
            <span class="min-w-0 truncate" :title="item.binding.device">{{ item.binding.device }}</span>
            <time class="shrink-0 tabular-nums" :datetime="item.deadline">{{ new Date(item.deadline).toLocaleTimeString() }} 到期</time>
          </div>
          </div>
        </div>
        <footer class="flex flex-wrap items-center gap-2 px-5 py-3">
          <Button :disabled="!!busy" variant="ghost" size="sm" class="rounded-full text-muted-foreground hover:text-destructive" @click="decide(item, 'deny')">拒绝</Button>
          <div class="ml-auto flex flex-wrap gap-2">
            <Button :disabled="!!busy" variant="outline" size="sm" class="rounded-full bg-background" @click="decide(item, 'take_over')">人工接管</Button>
            <Button :disabled="!!busy" size="sm" class="rounded-full px-4" @click="decide(item, 'allow_once')">允许一次</Button>
          </div>
        </footer>
      </article>
      <p v-if="standalone && !pending.length" class="px-5 pb-5 text-sm text-muted-foreground">暂无待审核操作</p>
    </div>
  </section>
</template>
