<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useDeviceStore } from '@/stores/devices'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const props = defineProps<{ serial: string | null }>()
const emit = defineEmits<{ close: [] }>()
const store = useDeviceStore()
const device = computed(() => store.devices.find(d => d.serial === props.serial))
const draft = ref(''), saving = ref(false), error = ref('')
let generation = 0
watch(() => props.serial, () => {
  generation++
  saving.value = false
  error.value = ''
  draft.value = device.value ? store.displayName(device.value) : ''
})
async function save() {
  if (saving.value || !props.serial) return
  const current = generation
  const serial = props.serial
  saving.value = true
  error.value = ''
  try {
    await store.saveAlias(serial, draft.value)
    if (current === generation) emit('close')
  }
  catch (cause) {
    if (current === generation) error.value = cause instanceof Error ? cause.message : '保存失败'
  }
  finally { if (current === generation) saving.value = false }
}
</script>

<template>
  <Dialog :open="serial !== null" @update:open="open => { if (!open) emit('close') }">
    <DialogContent class="sm:max-w-sm" :aria-describedby="undefined">
      <DialogHeader><DialogTitle>修改标识名</DialogTitle></DialogHeader>
      <form id="device-identifier" class="space-y-3" @submit.prevent="save">
        <label for="device-identifier-input" class="text-sm">标识名</label>
        <Input id="device-identifier-input" v-model="draft" :disabled="saving" autocomplete="off" />
        <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
      </form>
      <DialogFooter>
        <Button variant="outline" @click="emit('close')">取消</Button>
        <Button type="submit" form="device-identifier" class="bg-blue-600 text-white hover:bg-blue-700" :disabled="saving">保存</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
