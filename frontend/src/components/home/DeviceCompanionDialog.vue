<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { changeCompanion, getCompanionStatus, type CompanionStatus } from '@/api/companion'

const props = defineProps<{ serial: string | null }>()
const emit = defineEmits<{ close: [] }>()
const status = ref<CompanionStatus | null>(null)
const busy = ref(false), error = ref('')
let generation = 0, controller: AbortController | undefined
const label = computed(() => status.value?.update_available ? '待更新' : status.value?.ready ? '已启用' : status.value?.enabled ? '服务未就绪' : status.value?.installed ? '未启用' : '未安装')
async function load(action?: 'install' | 'enable' | 'disable') {
  if (!props.serial) return
  const serial = props.serial, current = ++generation
  controller?.abort()
  const request = new AbortController(); controller = request
  const timeout = setTimeout(() => request.abort(), action === 'install' ? 65000 : action ? 30000 : 15000)
  busy.value = true; error.value = ''
  try {
    const result = action ? await changeCompanion(serial, action, request.signal) : await getCompanionStatus(serial, request.signal)
    if (current === generation) { status.value = result; error.value = result.error ?? '' }
  }
  catch (cause) {
    if (current === generation) {
      error.value = request.signal.aborted ? '请求已结束，请刷新确认设备状态' : cause instanceof Error ? cause.message : '操作失败'
      if (action && !request.signal.aborted) {
        try {
          const checked = await getCompanionStatus(serial, AbortSignal.timeout(13000))
          if (current === generation) status.value = checked
        }
        catch (checkError) { console.warn('[companion] Verify action state failed', checkError) }
      }
    }
  }
  finally { clearTimeout(timeout); if (current === generation) busy.value = false }
}
watch(() => props.serial, serial => {
  generation++; controller?.abort(); status.value = null; error.value = ''; busy.value = false
  if (serial) void load()
})
onBeforeUnmount(() => { generation++; controller?.abort() })
</script>

<template>
  <Dialog :open="serial !== null" @update:open="open => { if (!open) emit('close') }">
    <DialogContent class="sm:max-w-sm" :aria-describedby="undefined">
      <DialogHeader><DialogTitle>设备助手</DialogTitle></DialogHeader>
      <div class="space-y-2">
        <div class="flex items-center justify-between text-sm"><span>Farm Companion</span><span>{{ status ? label : '读取中' }}</span></div>
        <p class="text-xs text-muted-foreground">启用后可通过无障碍服务读取页面控件，停用后使用 UI dump。</p>
        <p v-if="error" role="alert" class="break-words text-sm text-destructive">{{ error }}</p>
      </div>
      <DialogFooter>
        <Button variant="outline" :disabled="busy" @click="load()">刷新</Button>
        <Button v-if="status && (!status.installed || status.update_available)" class="bg-blue-600 text-white hover:bg-blue-700" :disabled="busy" @click="load('install')">{{ status.installed ? '更新' : '安装' }}</Button>
        <Button v-if="status?.installed && !status.enabled && !status.update_available" class="bg-blue-600 text-white hover:bg-blue-700" :disabled="busy" @click="load('enable')">启用</Button>
        <Button v-if="status?.enabled" variant="outline" :disabled="busy" @click="load('disable')">停用</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
