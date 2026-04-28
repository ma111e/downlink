<script setup lang="ts">
import { onMounted, onUnmounted, computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { RssIcon, WandSparkles, SettingsIcon, PlusIcon } from 'lucide-vue-next';
import { ListDigests } from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import FeedItem from '@/components/FeedItem.vue';
import { useFeeds } from '@/composables/useFeeds';
import { useArticles } from '@/composables/useArticles';
// feedStore no longer needed with query parameter approach
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { models } from "../../../wailsjs/go/models.ts";
import DigestListItem from '@/components/digests/DigestListItem.vue';
import { ScrollArea } from "@/components/ui/scroll-area";
import Digest = models.Digest;

const route = useRoute();
const router = useRouter();
const { feeds, loading: feedsLoading, error: feedsError, fetchFeeds } = useFeeds();
const { state, fetchArticleCounts, markAllArticlesAsRead, refreshCurrentList } = useArticles();
// Using route params directly instead of feedStore

// Tab state - explicitly set to 'feeds' by default
const currentMode = ref('feeds');

// Digest-related state
const digests = ref<Digest[]>([]);
const digestsLoading = ref(false);
const digestsError = ref<string | null>(null);

const refreshCounts = async () => {
  try {
    await fetchArticleCounts({
      query: route.query.q && typeof route.query.q === 'string' ? route.query.q : '',
      ...(route.query.from && typeof route.query.from === 'string' ? { start_date: route.query.from } : {}),
      ...(route.query.to && typeof route.query.to === 'string' ? { end_date: route.query.to } : {}),
    });
  } catch (err) {
    console.error('Failed to refresh article counts:', err);
  }
};

// Calculate unread count for "All" feed with active filters
const allUnreadCount = computed(() => {
  return Number(state.counts?.all_unread_count || 0);
});

// Get count of bookmarked articles with active filters
const bookmarkedCount = computed(() => {
  return Number(state.counts?.bookmarked_count || 0);
});

// Calculate filtered unread count for each feed
const getFilteredUnreadCount = (feedId: string) => {
  return Number(state.counts?.unread_by_feed?.[feedId] || 0);
};

// Fetch digests with optional filtering
const fetchDigests = async () => {
  try {
    digestsLoading.value = true;
    // Fetch up to 30 digests by default
    const limit = 30;
    digests.value = await ListDigests(limit);
    console.log('Fetched digests:', digests.value);
  } catch (err) {
    digestsError.value = 'Failed to load digests';
    console.error(err);
  } finally {
    digestsLoading.value = false;
  }
};

// Navigate to all articles
const showAllArticles = () => {
  // Navigate directly to all articles
  router.push('/all');
};

// Navigate to bookmarked articles
const showBookmarkedArticles = () => {
  // Use query parameter for bookmarked filter instead of a separate route
  router.push({ path: '/all', query: { bookmarked: 'true' } });
};

// Navigate to unread articles
const showUnreadArticles = () => {
  // Use query parameter for unread filter
  router.push({ path: '/all', query: { unread: 'true' } });
};

// Navigate to feed articles
const selectFeed = (feedId: string) => {
  const articleId = typeof route.query.articleId === 'string' ? route.query.articleId : '';
  const query = articleId.startsWith(`${feedId}:`) ? { articleId } : {};

  router.push({ path: `/feed/${feedId}`, query });
};

// Handle digest selection
const selectDigest = (digest: Digest) => {
  router.push(`/digest/${digest.id}`);
};

// Create new digest
const createNewDigest = () => {
  // Unselect the current digest and navigate to the digest creation page
  // This will reset the DigestDetail view to show the "generate a new digest" window
  router.push({
    path: '/digests',
    // Clear any potential query parameters that might be set
    query: {}
  });
};

// Navigate to settings page
const navigateToSettings = () => {
  router.push('/settings');
};

// Check if current route is for a specific feed
const isSelectedFeed = (feedId: string) => {
  return route.path === `/feed/${feedId}`;
};

// Check if we're on the all articles route
const isAllFeedSelected = computed(() => {
  return route.path === '/all';
});

// Check if we're on the bookmarked articles route
const isBookmarkedSelected = computed(() => {
  return route.query.bookmarked === 'true';
});

// Check if we're on the unread articles route
const isUnreadSelected = computed(() => {
  return route.query.unread === 'true';
});

// Check if a specific digest is currently selected
const isSelectedDigest = (digestId: string) => {
  return route.path === `/digest/${digestId}`;
};

// Mark all articles in a feed as read
const handleMarkAllRead = async (feedId: string) => {
  try {
    await markAllArticlesAsRead(feedId);
    await refreshCurrentList();
    await refreshCounts();
  } catch (err) {
    console.error('Failed to mark articles as read:', err);
  }
};

// Handle tab changes without using store
const handleTabChange = (value: string | number) => {
  const strVal = value.toString()

  currentMode.value = strVal;
  
  // Handle navigation based on selected tab
  if (strVal === 'digests') {
    router.push('/digests');
    fetchDigests(); // Fetch digests when switching to digests tab
  } else {
    // When switching back to feeds tab
    // Check if there's a feed Id in the current route
    const feedIdMatch = route.path.match(/\/feed\/(.+)/);
    if (feedIdMatch && feedIdMatch[1]) {
      // Stay on the current feed
      // No need to navigate, already on a feed page
    } else if (route.query.bookmarked === 'true') {
      // If we were on bookmarked view, stay there
      router.push({ path: '/all', query: { bookmarked: 'true' } });
    } else {
      // Default to all articles
      router.push('/all');
    }
  }
};

// On component mount
const handleReconnect = () => {
  fetchFeeds();
  refreshCounts();
};

onMounted(async () => {
  await fetchFeeds();
  await refreshCounts();

  // Initialize tab based on route, but default to feeds
  if (route.path.includes('/digests') || route.path.includes('/digest/')) {
    currentMode.value = 'digests';
    await fetchDigests();
  } else {
    // If on the root path, redirect to all feeds
    if (route.path === '/') {
      router.push('/all');
    }
  }

  window.addEventListener('downlink:reconnected', handleReconnect);
});

onUnmounted(() => {
  window.removeEventListener('downlink:reconnected', handleReconnect);
});


// Watch for route changes to update selected digest when navigating
watch(
  () => route.params.id,
  () => {
    if (route.path.includes('/digest/')) {
      // Make sure we're in digests mode when navigating to a digest
      currentMode.value = 'digests';
    }
  }
);

watch(
  () => [route.query.q, route.query.from, route.query.to],
  () => {
    refreshCounts();
  },
  { immediate: false }
);
</script>

<template>
  <div class="border-r border-gray-800 flex flex-col h-full max-h-screen overflow-y-auto w-full ">
    <div class="p-3">
      <Tabs default-value="feeds" :value="currentMode" @update:modelValue="handleTabChange" class="w-full">
        <TabsList class="grid grid-cols-2 w-full">
          <TabsTrigger value="feeds" class="cursor-pointer data-[state=active]:bg-gray-700">
            <div class="flex items-center justify-center">
              <RssIcon class="w-4 h-4 mr-1.5" />
              <span>Feeds</span>
            </div>
          </TabsTrigger>
          <TabsTrigger value="digests" class="cursor-pointer data-[state=active]:bg-indigo-700">
            <div class="flex items-center justify-center">
              <WandSparkles class="w-4 h-4 mr-1.5" />
              <span>Digests</span>
            </div>
          </TabsTrigger>
        </TabsList>
      </Tabs>
    </div>

    <!-- Feeds Content -->
    <div v-if="currentMode === 'feeds'" class="flex-1 flex flex-col">
      <div class="border-t border-gray-800">
        <div class="flex items-center justify-between p-4">
          <span class="section-label">FEEDS</span>
        </div>

        <div class="feed-list overflow-y-auto  overflow-x-hidden">
          <!-- Artificial "All" feed -->
          <div
            @click="showAllArticles"
            class="flex items-center justify-between px-4 py-2 my-1 mx-2 hover:bg-gray-800 cursor-pointer rounded-lg transition-all duration-200"
            :class="{ 'bg-gray-800': isAllFeedSelected }"
          >
            <div class="flex items-center truncate">
              <span class="truncate">All</span>
            </div>
            <div v-if="allUnreadCount > 0" class="bg-blue-600 rounded-full px-2 py-0.5 text-xs">
              {{ allUnreadCount }}
            </div>
          </div>

          <!-- Bookmarked articles feed -->
          <div
            @click="showBookmarkedArticles"
            class="flex items-center justify-between px-4 py-2 my-1 mx-2 hover:bg-gray-800 cursor-pointer rounded-lg transition-all duration-200"
            :class="{ 'bg-gray-800': isBookmarkedSelected }"
          >
            <div class="flex items-center truncate">
              <span class="truncate">Bookmarked</span>
            </div>
            <div v-if="bookmarkedCount > 0" class="bg-yellow-600 rounded-full px-2 py-0.5 text-xs">
              {{ bookmarkedCount }}
            </div>
          </div>

          <!-- Unread articles feed -->
          <div
            @click="showUnreadArticles"
            class="flex items-center justify-between px-4 py-2 my-1 mx-2 hover:bg-gray-800 cursor-pointer rounded-lg transition-all duration-200"
            :class="{ 'bg-gray-800': isUnreadSelected }"
          >
            <div class="flex items-center truncate">
              <span class="truncate">Unread</span>
            </div>
            <div v-if="allUnreadCount > 0" class="bg-blue-600 rounded-full px-2 py-0.5 text-xs">
              {{ allUnreadCount }}
            </div>
          </div>

          <FeedItem
            v-for="feed in feeds"
            :key="feed.id"
            :feed="feed"
            :selected="isSelectedFeed(feed.id)"
            :filtered-unread-count="getFilteredUnreadCount(feed.id)"
            @select="selectFeed(feed.id)"
            @markAllRead="handleMarkAllRead"
          />

          <div v-if="feeds.length === 0 && !feedsLoading" class="px-4 py-2 text-gray-500 text-sm">
            No feeds found
          </div>

          <div v-if="feedsLoading" class="px-4 py-2 text-gray-500 text-sm">
            Loading feeds...
          </div>

          <div v-if="feedsError" class="px-4 py-2 text-red-500 text-sm">
            {{ feedsError }}
          </div>
        </div>
      </div>
    </div>

    <!-- Digests Content -->
    <div v-else-if="currentMode === 'digests'" class="flex-1 flex flex-col">
      <div class="border-t border-gray-800">
        <div class="flex items-center justify-between p-4">
          <span class="section-label">DIGESTS</span>
          <div class="flex space-x-1">
            <button @click="createNewDigest" class="text-gray-400 hover:text-white" title="Create new digest">
              <PlusIcon class="w-5 h-5" />
            </button>
          </div>
        </div>

        <ScrollArea>
          <div v-if="digestsLoading" class="flex justify-center items-center p-4 text-gray-400">
            Loading digests...
          </div>

          <div v-else-if="!digests || digests.length === 0" class="flex justify-center items-center p-4 text-gray-400">
            No digests found
          </div>

          <div v-else>
            <DigestListItem
              v-for="digest in digests"
              :key="digest.id"
              :digest="digest"
              :is-selected="isSelectedDigest(digest.id)"
              @select="selectDigest(digest)"
            />
          </div>
        </ScrollArea>
      </div>
    </div>

    <div class="cursor-pointer px-4 py-2 my-4 mb-4 mx-2 hover:bg-gray-800 rounded-lg" @click="navigateToSettings">
      <button
        class="flex cursor-pointer items-center gap-2 w-full text-left"
      >
        <SettingsIcon class="w-4 h-4" />
        <span>Settings</span>
      </button>
    </div>
  </div>
</template>
