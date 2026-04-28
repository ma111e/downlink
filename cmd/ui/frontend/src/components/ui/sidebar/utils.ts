import type { ComputedRef, Ref } from 'vue'
import { createContext } from 'reka-ui'

export const SIdEBAR_COOKIE_NAME = 'sidebar:state'
export const SIdEBAR_COOKIE_MAX_AGE = 60 * 60 * 24 * 7
export const SIdEBAR_WIdTH = '16rem'
export const SIdEBAR_WIdTH_MOBILE = '18rem'
export const SIdEBAR_WIdTH_ICON = '3rem'
export const SIdEBAR_KEYBOARD_SHORTCUT = 'b'

export const [useSidebar, provideSidebarContext] = createContext<{
  state: ComputedRef<'expanded' | 'collapsed'>
  open: Ref<boolean>
  setOpen: (value: boolean) => void
  isMobile: Ref<boolean>
  openMobile: Ref<boolean>
  setOpenMobile: (value: boolean) => void
  toggleSidebar: () => void
}>('Sidebar')
