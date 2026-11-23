<script setup lang="ts">
import { ref, watch } from 'vue'
import {
  ChevronDown,
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
import AudioPanel from '@/components/player/AudioPanel.vue'
import WindowEffectsPanel from '@/components/player/WindowEffectsPanel.vue'

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

const menuOpen = ref(false)
const effectsExpanded = ref(false)
const effectsDialogOpen = ref(false)
const audioExpanded = ref(false)

// 展开的音量/窗口效果面板要在菜单关闭后复位，避免下次打开残留。
watch(menuOpen, open => {
  emit('update:open', open)
  if (open) return
  effectsExpanded.value = false
  audioExpanded.value = false
})
watch([effectsExpanded, effectsDialogOpen], ([expanded, dialog]) => emit('windowEffectsActive', expanded || dialog))
watch(audioExpanded, value => emit('update:audioOpen', value))

function toggleEffects() {
  effectsExpanded.value = !effectsExpanded.value
}

function toggleAudio() {
  audioExpanded.value = !audioExpanded.value
}

// 鼠标穿透确认框是独立的模态层，期间不能让更多菜单被外部按下关闭。
function onInteractOutside(event: Event) {
  if (effectsDialogOpen.value) event.preventDefault()
}

// 开启鼠标穿透后整个原生小窗不再接收输入，先收起菜单再交付穿透。
function handleClickThrough(enabled: boolean) {
  if (enabled) menuOpen.value = false
  emit('setClickThrough', enabled)
}
</script>

<template>
  <DropdownMenu v-model:open="menuOpen" :modal="false">
    <DropdownMenuTrigger as-child>
      <Button variant="ghost" size="icon" title="更多" aria-label="更多">
        <Ellipsis class="h-4 w-4" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent
      align="end"
      class="z-[9999] min-w-52"
      @interact-outside="onInteractOutside"
      @focus-outside="onInteractOutside"
    >
      <template v-if="nativePlayerWindow">
        <DropdownMenuItem class="justify-between" @select.prevent="toggleEffects">
          <span class="flex items-center gap-2"><Scaling class="h-4 w-4" />窗口效果</span>
          <ChevronDown class="h-4 w-4 shrink-0 text-muted-foreground transition-transform" :class="{ 'rotate-180': effectsExpanded }" />
        </DropdownMenuItem>
        <div v-if="effectsExpanded" class="px-2 pb-2" @pointerdown.stop @click.stop>
          <WindowEffectsPanel
            :opacity="opacity"
            :click-through="clickThrough"
            :shortcut="shortcut"
            :busy="windowBusy"
            @opacity="emit('setOpacity', $event)"
            @click-through="handleClickThrough"
            @restore="emit('restore')"
            @active-change="effectsDialogOpen = $event"
          />
        </div>
      </template>
      <DropdownMenuItem class="justify-between" @select.prevent="toggleAudio">
        <span class="flex items-center gap-2"><Volume2 class="h-4 w-4" />音量</span>
        <ChevronDown class="h-4 w-4 shrink-0 text-muted-foreground transition-transform" :class="{ 'rotate-180': audioExpanded }" />
      </DropdownMenuItem>
      <div v-if="audioExpanded" class="px-2 pb-2" @pointerdown.stop @click.stop>
        <AudioPanel
          :ready="audioReady"
          :muted="audioMuted"
          :volume="audioVolume"
          :title="audioTitle"
          @toggle="emit('toggleAudio')"
          @volume="emit('setAudioVolume', $event)"
        />
      </div>
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
