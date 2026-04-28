<script setup lang="ts">
import { computed } from 'vue';
import { useAnalysisQueueStore } from '@/stores/analysisQueueStore';

const props = defineProps<{
  articleId: string;
}>();

const queueStore = useAnalysisQueueStore();
const isAnalyzing = computed(() => queueStore.currentId === props.articleId && queueStore.isProcessing);
</script>

<template>
  <span v-if="isAnalyzing" class="analysis-indicator h-3 w-3 mr-1"></span>
</template>

<style scoped>
@keyframes analysisPulse {
  0%, 100% { opacity: 0.85; }
  50% { opacity: 0.45; }
}

.analysis-indicator {
  display: inline-block;
  background: #f97316;
  border-radius: 50%;
  position: relative;
  vertical-align: middle;
  animation: analysisPulse 1.2s ease-in-out infinite;
}

@media (prefers-reduced-motion: reduce) {
  .analysis-indicator {
    animation: none;
  }
}
</style>
