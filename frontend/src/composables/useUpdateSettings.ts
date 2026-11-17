import { ref } from 'vue'

export const UPDATE_SETTINGS_KEY = 'farm.update-settings'

export interface UpdateSettings {
  checkOnStartup: boolean
  autoUpdate: boolean
}

const DEFAULTS: UpdateSettings = {
  checkOnStartup: true,
  autoUpdate: false,
}

export function normalizeUpdateSettings(value: unknown): UpdateSettings {
  if (!value || typeof value !== 'object') return { ...DEFAULTS }
  const input = value as Partial<UpdateSettings>
  if (typeof input.checkOnStartup !== 'boolean') return { ...DEFAULTS }
  return {
    checkOnStartup: input.checkOnStartup,
    autoUpdate: input.checkOnStartup && input.autoUpdate === true,
  }
}

export function loadUpdateSettings(storage?: Pick<Storage, 'getItem'>): UpdateSettings {
  if (!storage) return { ...DEFAULTS }
  try {
    const raw = storage.getItem(UPDATE_SETTINGS_KEY)
    return raw ? normalizeUpdateSettings(JSON.parse(raw)) : { ...DEFAULTS }
  }
  catch {
    return { ...DEFAULTS }
  }
}

function browserStorage(): Pick<Storage, 'getItem' | 'setItem'> | undefined {
  return typeof localStorage === 'undefined' ? undefined : localStorage
}

const settings = ref<UpdateSettings>(loadUpdateSettings(browserStorage()))

function persist() {
  browserStorage()?.setItem(UPDATE_SETTINGS_KEY, JSON.stringify(settings.value))
}

export function useUpdateSettings() {
  function setCheckOnStartup(value: boolean) {
    settings.value = {
      checkOnStartup: value,
      autoUpdate: value ? settings.value.autoUpdate : false,
    }
    persist()
  }

  function setAutoUpdate(value: boolean) {
    settings.value = {
      checkOnStartup: settings.value.checkOnStartup,
      autoUpdate: settings.value.checkOnStartup && value,
    }
    persist()
  }

  return { settings, setCheckOnStartup, setAutoUpdate }
}

export function updateActionFor(result: { has_update: boolean; can_upgrade: boolean } | null | undefined, value: UpdateSettings) {
  if (!result?.has_update) return 'none' as const
  return value.autoUpdate && result.can_upgrade ? 'install' as const : 'prompt' as const
}
