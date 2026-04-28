<script setup lang="ts">
import { ShareIcon, BookOpenCheckIcon, BookOpenIcon, BookmarkIcon, BookmarkCheckIcon, ExternalLinkIcon, Settings2Icon } from 'lucide-vue-next';
import { ref, onMounted, onUnmounted, computed, defineProps, defineEmits } from 'vue';

const props = defineProps<{
  position: { x: number, y: number };
  actions: Record<string, string>;
}>();

const emit = defineEmits<{
  (e: 'action', action: string): void;
  (e: 'close'): void;
}>();

const selectedAction = ref<string | null>(null);

// Computed properties to determine icon state
const isBookmarked = computed(() => {
  return props.actions.bookmark.includes('Remove');
});

const isRead = computed(() => {
  return props.actions.read.includes('Unread');
});

// Register global event listeners
onMounted(() => {
  document.addEventListener('mousemove', handleMouseMove);
  document.addEventListener('mouseup', handleMouseUp);
});

// Clean up event listeners
onUnmounted(() => {
  document.removeEventListener('mousemove', handleMouseMove);
  document.removeEventListener('mouseup', handleMouseUp);
});

const handleMouseMove = (event: MouseEvent) => {
  // Calculate angle between initial position and current mouse position
  const dx = event.clientX - props.position.x;
  const dy = event.clientY - props.position.y;
  const distance = Math.sqrt(dx * dx + dy * dy);

  // Only select an action if the mouse has moved a certain distance from the center
  if (distance > 40) {
    // Calculate angle in degrees (0 to 360)
    let angle = Math.atan2(dy, dx) * (180 / Math.PI);
    if (angle < 0) angle += 360;

    // Determine selected action based on angle
    // We have 6 actions now, so each takes 60 degrees
    if (angle >= 330 || angle < 30) {
      selectedAction.value = 'share';
    } else if (angle >= 30 && angle < 90) {
      selectedAction.value = 'read';
    } else if (angle >= 90 && angle < 150) {
      selectedAction.value = 'bookmark';
    } else if (angle >= 150 && angle < 210) {
      selectedAction.value = 'analyze';
    } else if (angle >= 210 && angle < 270) {
      selectedAction.value = 'custom';
    } else {
      selectedAction.value = 'open';
    }
  } else {
    selectedAction.value = null;
  }
};

const handleMouseUp = () => {
  // If an action was selected, emit it
  if (selectedAction.value) {
    emit('action', selectedAction.value);
  } else {
    emit('close');
  }
};

</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center pointer-events-none">
    <div
      class="absolute bg-gray-700 rounded-full w-64 h-64 pointer-events-auto"
      :style="`top: ${position.y - 128}px; left: ${position.x - 128}px;`"
    >
      <!-- Center dot -->
      <div class="absolute top-1/2 left-1/2 w-4 h-4 bg-white rounded-full transform -translate-x-1/2 -translate-y-1/2"></div>

      <!-- Share option (right - 0 degrees) -->
      <div
        class="absolute top-[50%] left-[100%] transform translate-x-[-50%] -translate-y-1/2 flex items-center"
      >

        <div
          class="p-4 rounded-full transition-colors duration-200"
          :class="selectedAction === 'share' ? 'bg-blue-500' : 'bg-gray-800'"
        >
          <ShareIcon class="w-6 h-6 text-white" />
        </div>
      </div>

      <!-- Read option (bottom-right - 60 degrees) -->
      <div
        class="absolute top-[86.6%] left-[75%] transform -translate-x-1/2 -translate-y-1/2 flex items-center"
      >

        <div
          class="p-4 rounded-full transition-colors duration-200"
          :class="[
            selectedAction === 'read' ? 'bg-blue-500' : 'bg-gray-800',
            isRead && selectedAction !== 'read' ? 'border-2 border-blue-400' : ''
          ]"
        >
          <BookOpenCheckIcon v-if="isRead" class="w-6 h-6 text-white" />
          <BookOpenIcon v-else class="w-6 h-6 text-white" />
        </div>
      </div>

      <!-- Bookmark option (bottom-left - 120 degrees) -->
      <div
        class="absolute top-[86.6%] left-[25%] transform -translate-x-1/2 -translate-y-1/2 flex items-center"
      >

        <div
          class="p-4 rounded-full transition-colors duration-200"
          :class="[
            selectedAction === 'bookmark' ? 'bg-blue-500' : 'bg-gray-800',
            isBookmarked && selectedAction !== 'bookmark' ? 'border-2 border-yellow-400' : ''
          ]"
        >
          <BookmarkCheckIcon v-if="isBookmarked" class="w-6 h-6 text-white" />
          <BookmarkIcon v-else class="w-6 h-6 text-white" />
        </div>
      </div>

      <!-- Analyze option (left - 180 degrees) -->
      <div
        class="absolute top-[50%] left-[0%] transform translate-x-[-50%] -translate-y-1/2 flex items-center"
      >

        <div
          class="p-4 rounded-full transition-colors duration-200"
          :class="selectedAction === 'analyze' ? 'bg-blue-500' : 'bg-gray-800'"
        >
          <span class="w-6 h-6 text-white flex items-center justify-center">✨</span>
        </div>
      </div>

      <!-- Custom option (top-left - 240 degrees) -->
      <div
        class="absolute top-[13.4%] left-[25%] transform -translate-x-1/2 -translate-y-1/2 flex items-center"
      >

        <div
          class="p-4 rounded-full transition-colors duration-200"
          :class="selectedAction === 'custom' ? 'bg-blue-500' : 'bg-gray-800'"
        >
          <Settings2Icon class="w-6 h-6 text-white" />
        </div>
      </div>

      <!-- Open in web option (top-right - 300 degrees) -->
      <div
        class="absolute top-[13.4%] left-[75%] transform -translate-x-1/2 -translate-y-1/2 flex items-center"
      >

        <div
          class="p-4 rounded-full transition-colors duration-200"
          :class="selectedAction === 'open' ? 'bg-blue-500' : 'bg-gray-800'"
        >
          <ExternalLinkIcon class="w-6 h-6 text-white" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Prevent text selection when using the wheel */
.fixed {
  user-select: none;
}
</style>