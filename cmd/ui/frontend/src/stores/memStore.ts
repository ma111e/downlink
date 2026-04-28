import { createGlobalState } from '@vueuse/core'
import { shallowRef } from 'vue'
import { type Ref } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export const useMemStore = createGlobalState(() => {
  const isConnected: Ref<boolean> = shallowRef(false)

  EventsOn('connection:status', (connected: boolean) => {
    isConnected.value = connected
  })

  return { isConnected }
})
