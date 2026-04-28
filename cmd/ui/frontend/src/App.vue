<script setup lang="ts">
// import {useColorMode} from "@vueuse/core";
import { watch, ref } from 'vue'
import {Toaster} from "@/components/ui/toast";
import { useMemStore } from '@/stores/memStore'
import { useArticles } from '@/composables/useArticles'
import { useAnalysisQueueStore } from '@/stores/analysisQueueStore'

const { isConnected } = useMemStore()
const { refreshCurrentList } = useArticles()
const queueStore = useAnalysisQueueStore()

// Track previous connection state to detect reconnections
const wasConnected = ref(isConnected.value)

watch(isConnected, (connected) => {
  if (connected && !wasConnected.value) {
    // Reconnected — refresh all shared content and signal other components
    refreshCurrentList()
    queueStore.fetchStatus()
    window.dispatchEvent(new CustomEvent('downlink:reconnected'))
  }
  wasConnected.value = connected
})

// Needed here to init the theme palette
// useColorMode({
//   modes: {
//     lightOrange: 'lightOrange',
//     darkOrange: "darkOrange",
//     lightYellow: 'lightYellow',
//     darkYellow: 'darkYellow',
//     lightRose: 'lightRose',
//     darkRose: 'darkRose',
//     lightSlate: "lightSlate",
//     darkSlate: "darkSlate",
//     lightRed: "lightRed",
//     darkRed: "darkRed",
//   },
// })

</script>

<template>
  <div
      class="w-full h-screen bg-cross"
  >
    <div class="flex">
      <router-view/>
    </div>
  </div>
  <Toaster/>
</template>

<style>
body {
  font-family: Inconsolata, serif;
}

.select-content-fix {
  width: var(--reka-select-trigger-width);
  max-height: var(--reka-select-content-available-height);
}

.select-item-fix{
  /* total bar width - icon size - left padding - right padding */
  width: calc(var(--reka-select-trigger-width) - 14px - 2rem - 0.5rem);
}
</style>
