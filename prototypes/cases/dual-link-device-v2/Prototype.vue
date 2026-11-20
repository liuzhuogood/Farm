<script setup lang="ts">
import { computed, ref } from 'vue'
import { Battery, MessageCircle, Music2, Smartphone, Thermometer, Usb, Wifi } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import LinkCard from './components/LinkCard.vue'
import { MOCK_LINK_DEVICES, type MockLinkDevice } from './mockDevices'

const devices = ref<MockLinkDevice[]>(JSON.parse(JSON.stringify(MOCK_LINK_DEVICES)))
const view = ref<'home' | 'detail'>('home')
const selectedIdentity = ref(devices.value[0]!.identity)

const selected = computed(() => devices.value.find(device => device.identity === selectedIdentity.value) ?? devices.value[0]!)
const selectedOnline = computed(() => selected.value.links.some(link => link.state === 'online'))

function openDevice(device: MockLinkDevice) {
  selectedIdentity.value = device.identity
  view.value = 'detail'
}

const appIcon = computed(() => {
  if (selected.value.app.icon === 'wechat') return MessageCircle
  if (selected.value.app.icon === 'douyin') return Music2
  return Smartphone
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-center gap-2">
      <div class="inline-flex items-center gap-1 rounded-lg border bg-muted/40 p-1">
        <Button
          data-tab="home"
          size="sm"
          :variant="view === 'home' ? 'secondary' : 'ghost'"
          class="h-7 px-3 text-xs"
          @click="view = 'home'"
        >
          设备列表
        </Button>
        <Button
          data-tab="detail"
          size="sm"
          :variant="view === 'detail' ? 'secondary' : 'ghost'"
          class="h-7 px-3 text-xs"
          @click="view = 'detail'"
        >
          设备详情
        </Button>
      </div>
    </div>

    <section v-if="view === 'home'" class="grid grid-cols-1 justify-items-center gap-6 sm:grid-cols-2 lg:grid-cols-4">
      <LinkCard v-for="device in devices" :key="device.identity" :device="device" @open="openDevice(device)" />
    </section>

    <section v-else class="grid gap-6 lg:grid-cols-[minmax(0,360px)_minmax(0,1fr)]">
      <div class="mx-auto w-full max-w-[300px] rounded-3xl bg-[#0d0e11] p-4 text-white ring-1 ring-white/5 lg:max-w-none">
        <div class="flex items-center justify-between px-1 pb-3 text-[11px] font-mono text-neutral-400">
          <span class="flex items-center gap-1.5">
            <Battery class="h-3.5 w-3.5" />
            {{ selected.battery }}%
          </span>
          <span class="flex items-center gap-1.5">
            <Thermometer class="h-3.5 w-3.5" />
            {{ selected.batteryTemp.toFixed(1) }}°C
          </span>
        </div>
        <div class="flex h-[280px] flex-col items-center justify-center gap-4 rounded-2xl bg-black sm:h-auto sm:aspect-[9/16]">
          <div class="flex h-16 w-16 items-center justify-center rounded-2xl border border-sky-500/25 bg-sky-500/10 text-sky-400">
            <component :is="appIcon" class="h-8 w-8" />
          </div>
          <div class="text-center">
            <p class="text-sm font-medium text-neutral-100">{{ selected.app.name }}</p>
            <p v-if="selected.app.icon !== 'standby'" class="mt-1 font-mono text-[10px] text-neutral-500">{{ selected.app.pkg }}</p>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="space-y-1">
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="text-lg font-semibold">{{ selected.identifier }}</h2>
            <Badge variant="outline" class="gap-1.5 font-normal">
              <span class="h-1.5 w-1.5 rounded-full" :class="selectedOnline ? 'bg-emerald-500' : 'bg-neutral-400'" />
              {{ selectedOnline ? '在线' : '离线' }}
            </Badge>
          </div>
          <p class="text-xs text-muted-foreground">{{ selected.brand }} {{ selected.model }} · Android {{ selected.androidRelease }}</p>
        </div>

        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-sm font-medium">连接方式</CardTitle>
          </CardHeader>
          <CardContent class="grid gap-3 sm:grid-cols-2">
            <div
              v-for="link in selected.links"
              :key="link.kind"
              class="rounded-xl border p-3"
              :class="link.kind === selected.active ? 'border-blue-500/50 bg-blue-500/5' : 'border-border'"
            >
              <div class="flex items-center gap-2">
                <div
                  class="relative flex h-8 w-8 items-center justify-center rounded-lg border"
                  :class="link.kind === 'usb' ? 'border-cyan-500/25 bg-cyan-500/10 text-cyan-600 dark:text-cyan-400' : 'border-purple-500/25 bg-purple-500/10 text-purple-600 dark:text-purple-400'"
                >
                  <Usb v-if="link.kind === 'usb'" class="h-4 w-4" />
                  <Wifi v-else class="h-4 w-4" />
                  <span
                    class="absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full ring-2 ring-background"
                    :class="link.state === 'online' ? 'bg-emerald-500' : 'bg-neutral-400'"
                  />
                </div>
                <p
                  class="flex flex-1 items-center gap-1 text-[11px]"
                  :class="link.state === 'online' ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'"
                >
                  {{ link.state === 'online' ? '在线' : '已断开' }}
                </p>
                <Badge v-if="link.kind === selected.active" variant="secondary" class="h-5 px-1.5 text-[10px] font-normal">当前</Badge>
              </div>

              <p class="mt-3 truncate font-mono text-xs text-muted-foreground">{{ link.serial }}</p>
            </div>
          </CardContent>
        </Card>
      </div>
    </section>
  </div>
</template>
