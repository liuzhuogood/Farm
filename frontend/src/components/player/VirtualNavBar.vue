<script setup lang="ts">
import { ArrowLeft, Circle, GalleryHorizontal } from 'lucide-vue-next'

// 投屏画面下方的系统虚拟导航栏：与画面同宽，等同于手机屏幕下方的三大金刚键。
const emit = defineEmits<{
  back: []
  home: []
  recents: []
  drag: [event: MouseEvent]
}>()
</script>

<template>
  <div
    data-virtual-nav
    class="flex h-12 w-full shrink-0 items-center justify-around border-t border-white/15 bg-slate-900/85 px-6 shadow-lg backdrop-blur-md"
    @mousedown="emit('drag', $event)"
  >
    <button
      type="button"
      class="grid size-10 place-items-center rounded-lg text-white/85 transition hover:bg-white/15 hover:text-white"
      aria-label="切换任务"
      title="切换任务"
      @click.stop="emit('recents')"
    >
      <GalleryHorizontal class="h-4 w-4" />
    </button>
    <button
      type="button"
      class="grid size-10 place-items-center rounded-lg text-white/85 transition hover:bg-white/15 hover:text-white"
      aria-label="主页"
      title="主页"
      @click.stop="emit('home')"
    >
      <Circle class="h-4 w-4" />
    </button>
    <button
      type="button"
      class="grid size-10 place-items-center rounded-lg text-white/85 transition hover:bg-white/15 hover:text-white"
      aria-label="返回"
      title="返回"
      @click.stop="emit('back')"
    >
      <ArrowLeft class="h-5 w-5" />
    </button>
    <slot name="more" />
  </div>
</template>
