import { onUnmounted, ref, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import { toast } from 'vue-sonner'
import { createWakeUnlockTrigger, pressPowerWithUnlock, requestScreenUnlock, wakeAndUnlockOnOpen, wakeScreenIfOff } from './screenPower'

export function useScreenPower(serial: string, callbacks: {
  sendPowerKey: () => void
  onWake: () => void
  checkScreenState: () => Promise<void>
}) {
  const autoUnlockOnWake = useStorage('auto-unlock-on-wake', false)
  const powerBusy = ref(false)
  let activeController: AbortController | undefined
  let disposed = false

  async function runAction(action: (signal: AbortSignal) => Promise<void>): Promise<void> {
    if (disposed || powerBusy.value)
      return
    powerBusy.value = true
    const controller = new AbortController()
    activeController = controller
    let timedOut = false
    const timeout = window.setTimeout(() => {
      timedOut = true
      controller.abort()
    }, 15000)
    try {
      await action(controller.signal)
    }
    catch (error) {
      if (disposed || controller.signal.aborted && !timedOut)
        return
      console.warn('[wake-unlock] Power action failed', { serial, error })
      toast.error(timedOut ? '唤醒解锁请求超时，请检查设备连接' : error instanceof Error ? error.message : '唤醒解锁请求失败')
    }
    finally {
      window.clearTimeout(timeout)
      activeController = undefined
      powerBusy.value = false
    }
  }

  const trigger = createWakeUnlockTrigger(() => runAction(signal => requestScreenUnlock(serial, signal)))

  function observeScreenState(screenOn: boolean): Promise<void> {
    return trigger.observe(screenOn, autoUnlockOnWake.value)
  }

  function pressPower(): Promise<void> {
    return runAction(async signal => {
      const woke = await pressPowerWithUnlock(serial, autoUnlockOnWake.value, callbacks.sendPowerKey, signal)
      if (woke) {
        // 手动唤醒已完成同一解锁动作，标记本次亮屏以免定时检测重复请求。
        await trigger.observe(true, autoUnlockOnWake.value)
        callbacks.onWake()
      }
    })
  }

  function prepareOnOpen(): Promise<void> {
    return runAction(async signal => {
      await wakeAndUnlockOnOpen(serial, signal)
      await trigger.observe(true, autoUnlockOnWake.value)
      callbacks.onWake()
    })
  }

  /** 熄屏双击唤醒：只点亮屏幕，不解锁；自动解锁开关打开时再按开关注销锁屏。 */
  function wakeScreen(): Promise<void> {
    return runAction(async signal => {
      if (!await wakeScreenIfOff(serial, signal)) {
        // 状态滞后：屏幕其实已经亮着，刷新一下让界面退出熄屏提示。
        await callbacks.checkScreenState()
        return
      }
      await trigger.observe(true, autoUnlockOnWake.value)
      callbacks.onWake()
    })
  }

  watch(autoUnlockOnWake, enabled => {
    trigger.reset()
    if (enabled)
      void callbacks.checkScreenState()
    else
      activeController?.abort()
  })

  onUnmounted(() => {
    disposed = true
    activeController?.abort()
  })

  return { autoUnlockOnWake, powerBusy, pressPower, wakeScreen, observeScreenState, prepareOnOpen }
}
