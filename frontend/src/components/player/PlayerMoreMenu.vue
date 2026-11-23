<script setup lang="ts">
import {
  Ellipsis,
  ExternalLink,
  ListChecks,
  Maximize,
  Minimize,
  PictureInPicture2,
  Pin,
  PinOff,
  Power,
  RotateCw,
  Scaling,
  Settings,
  Volume2,
} from 'lucide-vue-next'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import AudioControl from '@/components/player/AudioControl.vue'
import WindowEffectsControl from '@/components/player/WindowEffectsControl.vue'

defineProps<{
  nativePlayerWindow: boolean
  isGui: boolean
  isAlwaysOnTop: boolean
  showPip: boolean
  pipSupported: boolean
  pipActive: boolean
  powerBusy: boolean
  fullscreen: boolean
  audioReady: boolean
  audioMuted: boolean
  audioVolume: number
  audioTitle: string
  opacity: number
  clickThrough: boolean
  shortcut: string | null
  windowBusy: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'update:alwaysOnTop': []
  'update:pip': []
  'update:audioOpen': [value: boolean]
  'toggleAudio': []
  'setAudioVolume': [value: number]
  'setOpacity': [value: number]
  'setClickThrough': [value: boolean]
  'restore': []
  'windowEffectsActive': [value: boolean]
  'pressPower': []
  'rotate': []
  'toggleFullscreen': []
  'openDeviceDetail': []
  'toggleTasks': []
  'openSettings': []
}>()
</script>

<template>
  <DropdownMenu :modal="false" @update:open="emit('update:open', $event)">
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="icon" title="更多" aria-label="更多">
        <Ellipsis class="h-4 w-4" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="z-[9999]">
      <DropdownMenuItem v-if="nativePlayerWindow" class="justify-between" @select.stop.prevent>
        <span class="flex items-center gap-2"><Scaling class="h-4 w-4" />窗口效果</span>
        <WindowEffectsControl
          :opacity="opacity"
          :click-through="clickThrough"
          :shortcut="shortcut"
          :busy="windowBusy"
          @opacity="emit('setOpacity', $event)"
          @click-through="emit('setClickThrough', $event)"
          @restore="emit('restore')"
          @active-change="emit('windowEffectsActive', $event)"
        />
      </DropdownMenuItem>
      <DropdownMenuItem class="justify-between" @select.stop.prevent>
        <span class="flex items-center gap-2"><Volume2 class="h-4 w-4" />音量</span>
        <AudioControl
          :ready="audioReady"
          :muted="audioMuted"
          :volume="audioVolume"
          :title="audioTitle"
          @toggle="emit('toggleAudio')"
          @volume="emit('setAudioVolume', $event)"
          @update:open="emit('update:audioOpen', $event)"
        />
      </DropdownMenuItem>
      <DropdownMenuItem v-if="isGui" @select="emit('update:alwaysOnTop')">
        <PinOff v-if="isAlwaysOnTop" class="h-4 w-4" />
        <Pin v-else class="h-4 w-4" />
        {{ isAlwaysOnTop ? '取消置顶' : '保持窗口置顶' }}
      </DropdownMenuItem>
      <DropdownMenuItem @select="emit('openDeviceDetail')"><ExternalLink class="h-4 w-4" />设备详情</DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem :disabled="powerBusy" @select="emit('pressPower')">
        <Power class="h-4 w-4" />电源键
      </DropdownMenuItem>
      <DropdownMenuItem @select="emit('rotate')">
        <RotateCw class="h-4 w-4" />横竖屏
      </DropdownMenuItem>
      <DropdownMenuItem @select="emit('toggleFullscreen')">
        <Minimize v-if="fullscreen" class="h-4 w-4" />
        <Maximize v-else class="h-4 w-4" />{{ fullscreen ? '退出全屏' : '全屏' }}
      </DropdownMenuItem>
      <DropdownMenuItem v-if="showPip" :disabled="!pipSupported" @select="emit('update:pip')">
        <PictureInPicture2 class="h-4 w-4" />{{ pipActive ? '退出画中画' : '画中画' }}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem @select="emit('toggleTasks')"><ListChecks class="h-4 w-4" />任务情况</DropdownMenuItem>
      <DropdownMenuItem @select="emit('openSettings')"><Settings class="h-4 w-4" />设置</DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
