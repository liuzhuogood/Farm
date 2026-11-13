<script setup lang="ts">
import { Camera, Film, Globe, Image, Layers, LayoutGrid, MapPin, Music, Phone, Settings, ShoppingBag, Tv } from 'lucide-vue-next'
withDefaults(defineProps<{ iconType?: string, sizeClass?: string }>(), { sizeClass: 'size-7.5' })
</script>
<template>
  <!-- 微信图标 -->
  <svg
    v-if="iconType === 'wechat'"
    :class="[sizeClass, 'text-[#07C160]']"
    viewBox="0 0 24 24"
    fill="currentColor"
  >
    <path d="M8.5 2C4.36 2 1 4.91 1 8.5c0 2.05 1.07 3.88 2.76 5.11L3 17.5l4.28-1.43c.4.08.8.13 1.22.13.23 0 .45-.02.67-.04-.26-.67-.42-1.39-.42-2.16 0-3.59 3.36-6.5 7.5-6.5.64 0 1.26.07 1.84.2C16.84 4.54 13.06 2 8.5 2zm-2.25 4a1.25 1.25 0 110 2.5 1.25 1.25 0 010-2.5zm4.5 0a1.25 1.25 0 110 2.5 1.25 1.25 0 010-2.5zM15.5 8.5c-3.59 0-6.5 2.46-6.5 5.5s2.91 5.5 6.5 5.5c.36 0 .71-.03 1.05-.09L20 20.5l-.9-3.08C20.9 16.38 22 14.8 22 13c0-3.04-2.91-5.5-6.5-5.5zm-2 3.5a1 1 0 110 2 1 1 0 010-2zm4 0a1 1 0 110 2 1 1 0 010-2z" />
  </svg>
  <Settings v-else-if="iconType === 'settings'" :class="sizeClass" />
  <LayoutGrid v-else-if="iconType === 'launcher'" :class="sizeClass" />
  <Camera v-else-if="iconType === 'camera'" :class="sizeClass" />
  <Image v-else-if="iconType === 'gallery'" :class="sizeClass" />
  <Phone v-else-if="iconType === 'phone'" :class="sizeClass" />
  <Globe v-else-if="iconType === 'browser'" :class="sizeClass" />
  <Film v-else-if="iconType === 'kuaishou' || iconType === 'douyin'" :class="sizeClass" />
  <Tv v-else-if="iconType === 'bilibili' || iconType === 'iqiyi' || iconType === 'qqvideo' || iconType === 'youku'" :class="sizeClass" />
  <ShoppingBag v-else-if="iconType === 'alipay' || iconType === 'taobao' || iconType === 'jd' || iconType === 'pinduoduo' || iconType === 'market'" :class="sizeClass" />
  <Music v-else-if="iconType === 'neteasemusic' || iconType === 'qqmusic'" :class="sizeClass" />
  <MapPin v-else-if="iconType === 'amap' || iconType === 'baidumap'" :class="sizeClass" />
  <Layers v-else :class="sizeClass" />
</template>
