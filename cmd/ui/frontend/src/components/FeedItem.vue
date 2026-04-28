<script setup lang="ts">
import { computed, ref } from 'vue';
import { models } from "../../wailsjs/go/models.ts";
import { MoreVertical, CheckCircle, XCircle } from 'lucide-vue-next';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator
} from '@/components/ui/dropdown-menu';
import { GetConfig, SaveConfig } from "../../wailsjs/go/downlinkclient/DownlinkClient";
import { useFeeds } from '@/composables/useFeeds';
import Feed = models.Feed;

const props = defineProps<{
  feed: Feed;
  selected: boolean;
  filteredUnreadCount?: number; // Add prop for filtered unread count
}>();

const emit = defineEmits<{
  (e: 'select'): void;
  (e: 'markAllRead', feedId: string): void;
}>();

const { fetchFeeds } = useFeeds();
const isUpdating = ref(false);
const unreadCount = computed(() => props.filteredUnreadCount ?? 0);

// Mark all articles in this feed as read
const markAllAsRead = (event: Event) => {
  event.stopPropagation(); // Prevent feed selection
  emit('markAllRead', props.feed.id);
};

// Toggle feed enabled state
const toggleFeedEnabled = async (event: Event) => {
  event.stopPropagation(); // Prevent feed selection

  try {
    isUpdating.value = true;

    // Get current models
    const config = await GetConfig();

    // Find and update the feed in the models
    const feedIndex = config.feeds.findIndex(f =>
      f.url === props.feed.url && f.type === props.feed.type
    );

    if (feedIndex !== -1) {
      // Toggle enabled state
      config.feeds[feedIndex].enabled = !props.feed.enabled;

      // Save updated models
      await SaveConfig(config);

      // Refresh feeds list to reflect changes
      await fetchFeeds();
    }
  } catch (error) {
    console.error('Failed to toggle feed status:', error);
  } finally {
    isUpdating.value = false;
  }
};
</script>

<template>
  <div
    @click="emit('select')"
    class="flex items-center justify-between px-4 py-2 my-1 mx-2 hover:bg-gray-800 cursor-pointer rounded-lg transition-all duration-200"
    :class="{ 'bg-gray-800': selected, 'opacity-50': !feed.enabled }"
  >
    <div class="flex items-center truncate">
      <span class="truncate" :class="{ 'text-gray-400': !feed.enabled }">{{ feed.title }}</span>
    </div>

    <div class="flex items-center gap-1">
      <!-- Unread count badge -->
      <span v-if="unreadCount > 0" class="bg-blue-600 rounded-full px-2 py-0.5 text-xs">
        {{ unreadCount }}
      </span>

      <DropdownMenu>
        <DropdownMenuTrigger
          @click.stop=""
          class="p-1 text-gray-400 hover:text-white rounded-full hover:bg-gray-700"
          :disabled="isUpdating"
        >
          <MoreVertical class="w-4 h-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          side="right"
        >
          <DropdownMenuItem
            @click="markAllAsRead"
            :disabled="unreadCount === 0"
          >
            Mark all as read
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <DropdownMenuItem
            @click="toggleFeedEnabled"
            :disabled="isUpdating"
          >
            <div class="flex items-center">
              <CheckCircle v-if="!feed.enabled" class="w-4 h-4 mr-2 text-green-500" />
              <XCircle v-else class="w-4 h-4 mr-2 text-red-500" />
              {{ feed.enabled ? 'Disable feed' : 'Enable feed' }}
            </div>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </div>
</template>
