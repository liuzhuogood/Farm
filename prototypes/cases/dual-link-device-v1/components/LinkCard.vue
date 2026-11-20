<script setup lang="ts">
import { computed } from 'vue'
import {
  Battery,
  MessageCircle,
  Music2,
  Smartphone,
  Thermometer,
  Usb,
  Wifi,
} from 'lucide-vue-next'
import type { DeviceLink, MockLinkDevice } from '../mockDevices'

const props = defineProps<{ device: MockLinkDevice }>()
const emit = defineEmits<{ (e: 'open'): void }>()

const isOnline = computed(() => props.device.links.some(link => link.state === 'online'))

const CHIP: Record<'usb' | 'wifi', { active: string; standby: string; offline: string }> = {
  usb: {
    active: 'border-cyan-500/35 bg-cyan-500/15 text-cyan-300',
    standby: 'border-white/15 bg-transparent text-neutral-400',
    offline: 'border-white/10 bg-transparent text-neutral-600',
  },
  wifi: {
    active: 'border-purple-500/35 bg-purple-500/15 text-purple-300',
    standby: 'border-white/15 bg-transparent text-neutral-400',
    offline: 'border-white/10 bg-transparent text-neutral-600',
  },
}

function chipClass(link: DeviceLink) {
  const styles = CHIP[link.kind]
  if (link.state === 'offline') return styles.offline
  return link.kind === props.device.active ? styles.active : styles.standby
}

function chipTitle(link: DeviceLink) {
  const name = link.kind === 'usb' ? 'USB' : '无线'
  if (link.state === 'offline') return `${name} 连接已断开`
  return link.kind === props.device.active ? `${name}（当前连接方式）` : name
}

const appIcon = computed(() => {
  if (props.device.app.icon === 'wechat') return MessageCircle
  if (props.device.app.icon === 'douyin') return Music2
  return Smartphone
})
</script>

<template>
  <div class="flex items-center justify-center w-full select-none">
    <div
      class="group relative flex flex-col w-[270px] h-[508px] overflow-hidden bg-[#0d0e11] text-white rounded-[36px] p-2 border-[3px] shadow-xl shadow-black/60 transition-all duration-200 hover:-translate-y-1 cursor-pointer"
      :class="isOnline ? 'border-neutral-800 hover:border-neutral-700 ring-1 ring-white/5' : 'border-neutral-900 opacity-55 grayscale'"
      @click="emit('open')"
    >
      <div class="absolute -left-[3px] top-18 h-8 w-[3px] rounded-r-sm bg-neutral-700" />
      <div class="absolute -left-[3px] top-28 h-8 w-[3px] rounded-r-sm bg-neutral-700" />
      <div class="absolute -right-[3px] top-22 h-10 w-[3px] rounded-l-sm bg-neutral-700" />

      <div class="relative flex flex-col justify-between overflow-hidden bg-black rounded-[28px] p-3 flex-1">
        <div class="relative z-10 flex flex-col gap-2 shrink-0">
          <div class="mx-auto flex items-center justify-center">
            <div class="h-4 rounded-full bg-black/95 px-3 flex items-center gap-1.5 border border-white/15 shadow-inner" :title="device.model">
              <span class="h-2 w-2 rounded-full bg-slate-900 border border-slate-700/80 flex items-center justify-center">
                <span class="h-0.5 w-0.5 rounded-full bg-blue-400/60" />
              </span>
              <span class="text-[9px] font-mono text-slate-400 font-semibold tracking-wider scale-90">
                {{ device.brand }}
              </span>
            </div>
          </div>

          <div class="flex items-center justify-between text-[11px] pt-0.5 px-1.5">
            <div class="flex items-center gap-1.5">
              <span
                v-for="link in device.links"
                :key="link.kind"
                class="relative flex h-6 w-6 shrink-0 items-center justify-center rounded-md border"
                :class="chipClass(link)"
                :title="chipTitle(link)"
                :aria-label="chipTitle(link)"
              >
                <Usb v-if="link.kind === 'usb'" class="h-3.5 w-3.5" />
                <Wifi v-else class="h-3.5 w-3.5" />
                <span
                  class="absolute -right-0.5 -top-0.5 h-1.5 w-1.5 rounded-full ring-2 ring-black"
                  :class="link.state === 'online' ? 'bg-emerald-400' : 'bg-neutral-600'"
                />
                <span v-if="link.state === 'offline'" class="absolute h-px w-4 rotate-45 bg-neutral-500" />
              </span>
            </div>

            <div class="flex items-center gap-2 font-mono text-slate-400">
              <div class="flex items-center gap-0.5 text-[10px]">
                <Thermometer class="h-3 w-3" />
                <span>{{ isOnline ? `${device.batteryTemp.toFixed(1)}°C` : '--' }}</span>
              </div>
              <div class="flex items-center gap-1 text-[10px]">
                <span>{{ isOnline ? `${device.battery}%` : '--' }}</span>
                <Battery class="h-3.5 w-3.5" />
              </div>
            </div>
          </div>
        </div>

        <div v-if="isOnline" class="relative my-auto flex flex-col items-center justify-center py-4 text-center">
          <div class="flex items-center justify-center border shadow-lg rounded-2xl h-15 w-15" :class="device.app.icon === 'wechat' ? 'bg-[#07c160]/12 border-[#07c160]/25 text-[#07c160]' : device.app.icon === 'douyin' ? 'bg-orange-500/12 border-orange-500/25 text-orange-400' : 'bg-sky-500/12 border-sky-500/25 text-sky-400'">
            <component :is="appIcon" class="h-7 w-7" />
          </div>
          <div class="mt-3 w-full px-2">
            <h4 class="truncate text-sm font-medium tracking-tight text-neutral-100">
              {{ device.app.name }}
            </h4>
            <p v-if="device.app.icon !== 'standby'" class="truncate font-mono text-[10px] text-neutral-500 mt-0.5">
              {{ device.app.pkg }}
            </p>
          </div>
        </div>

        <div v-else class="relative my-auto flex flex-col items-center justify-center py-4 text-center">
          <div class="flex items-center justify-center rounded-2xl h-15 w-15 bg-neutral-900/60 border border-neutral-800 text-neutral-600">
            <Smartphone class="h-7 w-7" />
          </div>
          <div class="mt-3 w-full px-2">
            <h4 class="text-sm font-medium text-neutral-500">
              设备离线
            </h4>
          </div>
        </div>

        <div class="relative z-10 shrink-0 border-t border-neutral-800/80 pt-2 pb-0.5 flex flex-col gap-1.5" @click.stop>
          <div class="flex items-center justify-between gap-1.5 px-0.5">
            <div class="min-w-0 flex-1 truncate">
              <span class="font-medium text-neutral-200 truncate text-xs">{{ device.identifier }}</span>
              <p class="mt-0.5 truncate font-mono text-[9px] text-neutral-500">
                {{ device.model }} · {{ device.identity.replace('hw:', '') }}
              </p>
            </div>
          </div>
          <div class="mx-auto rounded-full bg-neutral-800 h-1 w-14 mt-1 mb-0.5" />
        </div>
      </div>
    </div>
  </div>
</template>
