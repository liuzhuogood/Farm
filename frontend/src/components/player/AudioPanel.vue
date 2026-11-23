<script setup lang="ts">
import { Volume2, VolumeX } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Slider } from '@/components/ui/slider'

defineProps<{ ready: boolean, muted: boolean, volume: number, title: string }>()
const emit = defineEmits<{ toggle: [], volume: [value: number] }>()

function updateVolume(values: number[] | undefined) {
  if (values?.[0] !== undefined) emit('volume', values[0] / 100)
}
</script>

<template>
  <div class="flex items-center gap-3" :title="title">
    <Button variant="ghost" size="icon" :disabled="!ready" :aria-label="muted ? '播放声音' : '静音'" @click="emit('toggle')">
      <VolumeX v-if="muted" class="h-4 w-4" /><Volume2 v-else class="h-4 w-4" />
    </Button>
    <Slider
      :model-value="[Math.round(volume * 100)]"
      :min="0"
      :max="100"
      :step="1"
      :disabled="!ready"
      aria-label="投屏音量"
      @update:model-value="updateVolume"
    />
    <span class="w-9 shrink-0 text-right text-xs tabular-nums">{{ Math.round(volume * 100) }}%</span>
  </div>
</template>
