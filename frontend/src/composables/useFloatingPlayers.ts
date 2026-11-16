import { ref } from 'vue'

// Web 端内嵌浮动小窗列表：首页与设备详情页共享同一份状态。
const floatingSerials = ref<string[]>([])

export function useFloatingPlayers() {
  function open(serial: string) {
    if (!serial || floatingSerials.value.includes(serial))
      return
    floatingSerials.value = [...floatingSerials.value, serial]
  }

  function close(serial: string) {
    floatingSerials.value = floatingSerials.value.filter(item => item !== serial)
  }

  return { floatingSerials, open, close }
}
