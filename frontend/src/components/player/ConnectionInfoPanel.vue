<script setup lang="ts">
import { describeChannel, describeMode, type ConnectionInfo } from '@/scrcpy/connection'

const props = defineProps<{ info: ConnectionInfo, live: boolean }>()

function rtt(info: ConnectionInfo) {
  return info.rttMs === undefined ? null : `${Math.round(info.rttMs)} ms`
}
</script>

<template>
  <div class="space-y-1.5 text-xs">
    <div class="flex items-center justify-between gap-4">
      <span class="text-muted-foreground">通道</span>
      <span class="font-mono tabular-nums">{{ describeChannel(props.info) }}</span>
    </div>
    <div class="flex items-center justify-between gap-4">
      <span class="text-muted-foreground">传输</span>
      <span>{{ live ? describeMode(props.info) : '未连接' }}</span>
    </div>
    <div v-if="live && rtt(props.info)" class="flex items-center justify-between gap-4">
      <span class="text-muted-foreground">往返时延</span>
      <span class="font-mono tabular-nums">{{ rtt(props.info) }}</span>
    </div>
  </div>
</template>
