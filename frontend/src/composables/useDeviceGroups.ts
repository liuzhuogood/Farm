import { createSharedComposable, useStorage } from '@vueuse/core'

export interface DeviceGroup {
  id: string
  name: string
  serials: string[]
}

export const DEVICE_GROUPS_KEY = 'farm.device-groups'

function normalizeGroups(raw: unknown): DeviceGroup[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter((item): item is DeviceGroup => Boolean(item) && typeof item.id === 'string' && typeof item.name === 'string' && Array.isArray(item.serials))
    .map(item => ({
      id: item.id,
      name: item.name,
      serials: item.serials.filter((serial): serial is string => typeof serial === 'string'),
    }))
}

/**
 * 设备分组：在本机保存分组名称与成员 serial。
 * 分组只记录 serial，选中时再由首页按当前可选设备过滤，离线或隐藏的设备不会被派发操作。
 */
export const useDeviceGroups = createSharedComposable(() => {
  const groups = useStorage<DeviceGroup[]>(DEVICE_GROUPS_KEY, [], undefined, {
    serializer: {
      read: raw => normalizeGroups(JSON.parse(raw || '[]')),
      write: value => JSON.stringify(value),
    },
  })

  function createGroup(name: string, serials: string[]): DeviceGroup | null {
    const trimmed = name.trim()
    const members = [...new Set(serials)]
    if (!trimmed || members.length === 0) return null
    const group: DeviceGroup = { id: crypto.randomUUID(), name: trimmed, serials: members }
    groups.value = [...groups.value, group]
    return group
  }

  function removeGroup(id: string) {
    groups.value = groups.value.filter(group => group.id !== id)
  }

  function updateGroupSerials(id: string, serials: string[]) {
    const members = [...new Set(serials)]
    if (members.length === 0) return
    groups.value = groups.value.map(group => group.id === id ? { ...group, serials: members } : group)
  }

  return { groups, createGroup, removeGroup, updateGroupSerials }
})
