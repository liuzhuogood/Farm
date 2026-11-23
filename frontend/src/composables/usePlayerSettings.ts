import { computed } from 'vue'
import type { Ref } from 'vue'
import { createSharedComposable, useStorage } from '@vueuse/core'

export type OpenMode = 'floating' | 'page'
export type FloatingSize = 'compact' | 'normal' | 'large'
/** 点击/滑动注入模式：sdk = scrcpy 注入（默认），adb = adb shell input，hid = UHID 虚拟触摸屏 */
export type InjectMode = 'sdk' | 'adb' | 'hid'
export type HomeViewMode = 'deck' | 'phone'
export const HOME_VIEW_MODE_KEY = 'farm.view_mode'
export const HOME_PHONE_SCALE_KEY = 'home.phone_scale'
export const HOME_QUALITY_KEY = 'home.quality'

export const HOME_PHONE_SCALE_MIN = 10
export const HOME_PHONE_SCALE_MAX = 200
export const HOME_PHONE_SCALE_DEFAULT = 100
export const HOME_PHONE_BASE_WIDTH = 270
export const HOME_QUALITY_MIN = 50
export const HOME_QUALITY_MAX = 100
export const HOME_QUALITY_DEFAULT = 90
export const HOME_BITRATE_MIN = 2
export const HOME_BITRATE_MAX = 16
export const HOME_BITRATE_DEFAULT = 8

function normalizeVirtualNavs(raw: unknown): Record<string, boolean> {
  if (!raw || typeof raw !== 'object') return {}
  const entries = Object.entries(raw as Record<string, unknown>)
    .filter(([serial, value]) => Boolean(serial) && value === true)
  return Object.fromEntries(entries) as Record<string, boolean>
}

export function normalizeHomeViewMode(raw: unknown): HomeViewMode {
  return raw === 'phone' ? 'phone' : 'deck'
}

export function clampPercentage(value: unknown, min: number, max: number, fallback: number): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed))
    return fallback
  return Math.min(max, Math.max(min, Math.round(parsed)))
}

// Share one storage subscription set across cards, windows and settings in this WebView.
export const usePlayerSettings = createSharedComposable(() => {
  const openMode = useStorage<OpenMode>('player-open-mode', 'floating')
  const floatingSize = useStorage<FloatingSize>('player-floating-size', 'normal')
  const autoAudio = useStorage<boolean>('player-auto-audio', false)
  const bitrate = useStorage<number>('player-bitrate', 4000000)
  const maxFps = useStorage<number>('player-max-fps', 30)
  const injectMode = useStorage<InjectMode>('player-inject-mode', 'sdk')
  const homeViewMode = useStorage<HomeViewMode>(HOME_VIEW_MODE_KEY, 'deck', undefined, {
    serializer: {
      read: raw => normalizeHomeViewMode(raw),
      write: value => value,
    },
  })
  const homePhoneScale = useStorage<number>(
    HOME_PHONE_SCALE_KEY,
    HOME_PHONE_SCALE_DEFAULT,
    undefined,
    {
      serializer: {
        read: raw => clampPercentage(raw, HOME_PHONE_SCALE_MIN, HOME_PHONE_SCALE_MAX, HOME_PHONE_SCALE_DEFAULT),
        write: value => String(value),
      },
    },
  )
  const homeQuality = useStorage<number>(HOME_QUALITY_KEY, HOME_QUALITY_DEFAULT, undefined, {
    serializer: {
      read: raw => clampPercentage(raw, HOME_QUALITY_MIN, HOME_QUALITY_MAX, HOME_QUALITY_DEFAULT),
      write: value => String(value),
    },
  })
  const homeBitrate = useStorage<number>('home.bitrate', HOME_BITRATE_DEFAULT, undefined, {
    serializer: {
      read: raw => clampPercentage(raw, HOME_BITRATE_MIN, HOME_BITRATE_MAX, HOME_BITRATE_DEFAULT),
      write: value => String(value),
    },
  })

  // 同步复用现有键值，确保全面向后兼容
  const autoUnlockOnWake = useStorage<boolean>('auto-unlock-on-wake', true)
  const clipboardSync = useStorage<boolean>('clipboard-sync', true)
  const touchVisible = useStorage<boolean>('show-touch-overlay', true)
  const boxesVisible = useStorage<boolean>('show-template-boxes', true)
  // 虚拟导航键按设备保存：有的设备需要，有的不需要，不同设备互不影响。
  const virtualNavs = useStorage<Record<string, boolean>>(
    'farm.player.virtual-nav',
    {},
    undefined,
    {
      serializer: {
        read: raw => normalizeVirtualNavs(JSON.parse(raw || '{}')),
        write: value => JSON.stringify(value),
      },
    },
  )

  function isVirtualNavVisible(serial: Ref<string>) {
    return computed(() => {
      const key = serial.value
      return key ? virtualNavs.value[key] === true : false
    })
  }

  function setVirtualNavVisible(serial: string, value: boolean) {
    if (!serial) return
    const next = { ...virtualNavs.value }
    if (value) next[serial] = true
    else delete next[serial]
    virtualNavs.value = next
  }

  return {
    openMode,
    floatingSize,
    autoAudio,
    bitrate,
    maxFps,
    injectMode,
    homeViewMode,
    homePhoneScale,
    homeQuality,
    homeBitrate,
    autoUnlockOnWake,
    clipboardSync,
    touchVisible,
    boxesVisible,
    isVirtualNavVisible,
    setVirtualNavVisible,
  }
})
