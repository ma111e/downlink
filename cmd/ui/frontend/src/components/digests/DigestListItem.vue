<script setup lang="ts">
import { computed } from 'vue';
import { models } from "../../../wailsjs/go/models.ts";
import Digest = models.Digest;
import { useFormatters } from '@/composables/useFormatters';

const { formatDate, convertNanoToHours } = useFormatters();

const props = defineProps<{
  digest: Digest;
  isSelected: boolean;
}>();

const emit = defineEmits<{
  (e: 'select', digest: Digest): void;
}>();


// Count successful provider results
const successfulProviders = computed(() => {
  // Add null check before filtering
  return props.digest.provider_results ? 
    props.digest.provider_results.filter(result => !result.error).length : 0;
});

// Handle click on the digest item
const handleClick = () => {
  emit('select', props.digest);
};
</script>

<template>
  <div
    class="p-4 border-b border-gray-800 hover:bg-gray-900 cursor-pointer"
    :class="{ 'bg-gray-900': isSelected }"
    @click="handleClick"
  >
    <!-- Title and blue detail on the same line -->
    <div class="flex items-center justify-between mb-2">
      <div class="font-medium">
        {{ formatDate(digest.created_at) }}
      </div>
      <div class="text-xs bg-blue-900 text-blue-300 px-2 py-1 rounded">
        {{ digest.article_count }} articles • {{ convertNanoToHours(digest.time_window) }}h
      </div>
    </div>

    <div class="flex justify-between text-xs text-gray-400 mt-3">
      <div>
        {{ successfulProviders }}/{{ digest.provider_results ? digest.provider_results.length : 0 }} providers
      </div>
    </div>
  </div>
</template>