<script setup lang="ts">
import { Volume2, VolumeX } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import AudioPanel from '@/components/player/AudioPanel.vue'

defineProps<{ ready: boolean, muted: boolean, volume: number, title: string }>()
const emit = defineEmits<{ toggle: [], volume: [value: number], 'update:open': [open: boolean] }>()
</script>

<template>
  <Popover :modal="false" @update:open="emit('update:open', $event)">
    <PopoverTrigger as-child>
      <Button variant="ghost" size="icon" :disabled="!ready" :title="title" aria-label="音量控制">
        <VolumeX v-if="muted" class="h-5 w-5" /><Volume2 v-else class="h-5 w-5" />
      </Button>
    </PopoverTrigger>
    <PopoverContent class="w-60" align="end">
      <AudioPanel
        :ready="ready"
        :muted="muted"
        :volume="volume"
        :title="title"
        @toggle="emit('toggle')"
        @volume="emit('volume', $event)"
      />
    </PopoverContent>
  </Popover>
</template>
