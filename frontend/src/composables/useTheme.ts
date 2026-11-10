import { useColorMode } from '@vueuse/core'

export const theme = useColorMode({ storageKey: 'farm-theme', initialValue: 'auto', emitAuto: true, disableTransition: true })
