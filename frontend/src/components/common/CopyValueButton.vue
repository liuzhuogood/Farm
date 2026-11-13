<script setup lang="ts">
import { Check, Copy } from 'lucide-vue-next'
import { ref } from 'vue'
import { toast } from 'vue-sonner'

const props = withDefaults(defineProps<{
  value: string
  label?: string
  class?: string
}>(), {
  label: '内容',
  class: '',
})

const copied = ref(false)
let resetTimer: ReturnType<typeof setTimeout> | undefined

async function copyValue() {
  if (!props.value) return
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(props.value)
    }
    else {
      const area = document.createElement('textarea')
      area.value = props.value
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.append(area)
      area.select()
      const succeeded = document.execCommand('copy')
      area.remove()
      if (!succeeded) throw new Error('copy failed')
    }
    copied.value = true
    if (resetTimer) clearTimeout(resetTimer)
    resetTimer = setTimeout(() => { copied.value = false }, 1200)
  }
  catch {
    toast.error(`复制${props.label}失败`)
  }
}
</script>

<template>
  <button
    type="button"
    class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
    :class="props.class"
    :aria-label="`复制${props.label}`"
    :title="`复制${props.label}`"
    :disabled="!props.value"
    @click.stop="copyValue"
  >
    <Check v-if="copied" class="h-3.5 w-3.5 text-emerald-500" />
    <Copy v-else class="h-3.5 w-3.5" />
  </button>
</template>
