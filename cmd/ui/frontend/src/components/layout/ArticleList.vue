<script setup lang="ts">
import {computed, nextTick, onMounted, ref, watch} from 'vue';
import {useRouter, useRoute} from 'vue-router';
import {useInfiniteScroll, useVirtualList} from '@vueuse/core';
import {models} from "../../../wailsjs/go/models.ts";
import {GetAllArticleAnalyses} from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import SearchBar from '@/components/SearchBar.vue';
import ArticleListItem from '@/components/articles/ArticleListItem.vue';
import {useArticles} from '@/composables/useArticles';
import type {ArticleQueryInput} from '@/composables/useArticles';
import {useFeeds} from '@/composables/useFeeds';
import {useAnalysisQueueStore} from '@/stores/analysisQueueStore.ts';
// feedStore no longer needed with query parameter approach
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Popover, PopoverContent, PopoverTrigger} from "@/components/ui/popover";
import {RangeCalendar} from "@/components/ui/range-calendar";
import {BookmarkIcon, EyeIcon, CalendarIcon, ChevronDownIcon, SparklesIcon, XIcon} from "lucide-vue-next";
import type {DateRange} from 'reka-ui';
import Article = models.Article;
import {format} from 'date-fns';
import {CalendarDateTime} from '@internationalized/date';

const props = defineProps<{
  searchQuery?: string;
  unreadOnly?: boolean;
  feedId?: string;
  selectedArticleId?: string;
  bookmarkedOnly?: boolean;
}>()

const emit = defineEmits<{
  'article-selected': [articleId: string];
}>()

const router = useRouter();
const route = useRoute();
const {state, markArticleAsRead, loadInitialArticles, loadMoreArticles} = useArticles();
const {fetchFeeds} = useFeeds();
const queueStore = useAnalysisQueueStore();
// Using route params directly instead of feedStore
const localSearchQuery = ref(props.searchQuery || '');
const lastSelectedArticleId = ref<string | null>(null);

// Local state for filters (initialized from props/route)
const isUnreadOnly = ref(props.unreadOnly || false);
const isBookmarkedOnly = ref(props.bookmarkedOnly || false);
const dateRange = ref<DateRange>({ start: undefined, end: undefined });
const showDateRangePicker = ref(false);

// Computed property to determine effective feed Id
const effectiveFeedId = computed(() => {
  // If a feed Id is provided in props (from route), use that
  if (props.feedId) {
    return props.feedId;
  }
  // If a feed is specified in query params, use that
  if (route.query.feed && typeof route.query.feed === 'string') {
    return route.query.feed;
  }
  // Otherwise, no feed filter
  return null;
});

const currentArticles = computed(() => state.articles);
const currentArticleId = computed(() => (
  typeof route.query.articleId === 'string' ? route.query.articleId : undefined
));

// Update route with current filters
const updateRouteWithFilters = () => {
  const query: {
    unread?: string;
    bookmarked?: string;
    q?: string;
    from?: string;
    to?: string;
    feed?: string;
    articleId?: string;
  } = {
    ...(isUnreadOnly.value ? { unread: 'true' } : {}),
    ...(isBookmarkedOnly.value ? { bookmarked: 'true' } : {}),
    ...(localSearchQuery.value ? { q: localSearchQuery.value } : {}),
    ...(dateRange.value.start && dateRange.value.end ? {
      from: dateRange.value.start.toString(),
      to: (() => {
        // Create a date object for the end date and set it to end of day (23:59:59.999)
        const endDate = dateRange.value.end!.toDate('UTC');
        endDate.setHours(23, 59, 59, 999);
        // Convert back to the CalendarDateTime format expected in the URL
        return new CalendarDateTime(
          endDate.getFullYear(),
          endDate.getMonth() + 1,
          endDate.getDate(),
          23, 59, 59, 999
        ).toString();
      })(),
    } : {}),
    ...(effectiveFeedId.value ? { feed: effectiveFeedId.value } : {}),
    ...(currentArticleId.value ? { articleId: currentArticleId.value } : {})
  };
  
  // Preserve feed Id in path if we have one
  const path = props.feedId ? `/feed/${props.feedId}` : '/all';
  
  // Replace the current route with updated query params
  router.replace({ path, query });
};

// Format the date range for display
const formattedDateRange = computed(() => {
  if (dateRange.value.start && dateRange.value.end) {
    // Convert DateValue objects to JavaScript Date objects
    const startDateObj = dateRange.value.start;
    const endDateObj = dateRange.value.end;
    
    return `${format(startDateObj.toDate('UTC'), 'MMM d, yyyy')} - ${format(endDateObj.toDate('UTC'), 'MMM d, yyyy')}`;
  }
  return 'All time'; // Changed to indicate no date filter is applied
});

// Clear date range filter
const clearDateRange = () => {
  dateRange.value = { start: undefined, end: undefined };
  updateRouteWithFilters();
  showDateRangePicker.value = false;
};

// Update filters when date range changes
watch(
  () => dateRange.value,
  (newRange) => {
    if (newRange.start && newRange.end) {
      updateRouteWithFilters();
      showDateRangePicker.value = false;
    }
  }
);

// Toggle filters
const toggleUnreadFilter = () => {
  isUnreadOnly.value = !isUnreadOnly.value;
  updateRouteWithFilters();
};

const toggleBookmarkFilter = () => {
  isBookmarkedOnly.value = !isBookmarkedOnly.value;
  updateRouteWithFilters();
};

// Search articles as user types (debounced in SearchBar)
const handleSearch = () => {
  updateRouteWithFilters();
};

const updateDateRange = (newRange: DateRange) => {
  dateRange.value = newRange;
}; 

// Handle article selection
const selectArticle = async (article: Article) => {
  lastSelectedArticleId.value = article.id;

  if (currentArticleId.value === article.id) return;

  try {
    // Load article analyses in the background
    await GetAllArticleAnalyses(article.id).catch(err => {
      console.error('Failed to fetch article analyses:', err);
    });

    if (!article.read) {
      await markArticleAsRead(article.id);
      // No need to update local state as it's handled by the useArticles composable
    }

  // Check if we're in a digest view context
  const isDigestView = route.path.includes('/digest/');
  if (isDigestView) {
    // When in digest view, emit an event to notify parent components
    // instead of navigating away
    emit('article-selected', article.id);
    return;
  }

  // For non-digest views, proceed with normal navigation
  // Build the query params to preserve filters
  const query: {
    articleId: string;
    unread?: string;
    bookmarked?: string;
    q?: string;
    feed?: string;
    from?: string;
    to?: string;
  } = {
    articleId: article.id, // Add the article Id as a query parameter
    ...(isUnreadOnly.value ? { unread: 'true' } : {}),
    ...(isBookmarkedOnly.value ? { bookmarked: 'true' } : {}),
    ...(localSearchQuery.value ? { q: localSearchQuery.value } : {}),
    ...(dateRange.value.start && dateRange.value.end ? {
      from: dateRange.value.start.toString(),
      to: (() => {
        const endDate = dateRange.value.end!.toDate('UTC');
        endDate.setHours(23, 59, 59, 999);
        return new CalendarDateTime(
          endDate.getFullYear(),
          endDate.getMonth() + 1,
          endDate.getDate(),
          23, 59, 59, 999
        ).toString();
      })(),
    } : {})
  };

  // If we have a feed context, add a feed query param to preserve it
  if (props.feedId) {
    query.feed = props.feedId;
  } else if (route.query.feed && typeof route.query.feed === 'string') {
    query.feed = route.query.feed;
  }

  console.log('Selecting article with Id:', article.id);

  // Determine the correct path based on the current route
  let path = '/all';
  if (props.feedId) {
    path = `/feed/${props.feedId}`;
  } else if (route.path.includes('/feed/')) {
    path = route.path; // Keep the current feed path
  }

  // Navigate using query params only
  await router.push({
    path: path,
    query
  });

  // Manually emit the event to ensure consistent behavior
  emit('article-selected', article.id);
  } catch (error) {
    console.error('Error selecting article:', error);
  }
};

// Watch for changes in route query params
watch(
  () => route.query,
  (newQuery) => {
    isUnreadOnly.value = newQuery.unread === 'true';
    isBookmarkedOnly.value = newQuery.bookmarked === 'true';
    if (newQuery.q && typeof newQuery.q === 'string') {
      localSearchQuery.value = newQuery.q;
    } else {
      localSearchQuery.value = '';
    }

    // Handle date range params
    if (newQuery.from && newQuery.to &&
        typeof newQuery.from === 'string' &&
        typeof newQuery.to === 'string') {
      try {
        // Parse dates from URL parameters
        const from = new Date(newQuery.from);
        const to = new Date(newQuery.to);

        // For the UI date picker display, we want to show just the date (without time)
        // So regardless of the time in the URL parameter, we set it to the date only
        dateRange.value = {
          start: new CalendarDateTime(from.getFullYear(), from.getMonth() + 1, from.getDate()),
          end: new CalendarDateTime(to.getFullYear(), to.getMonth() + 1, to.getDate())
        };
      } catch (e) {
        dateRange.value = { start: undefined, end: undefined };
        console.error('Error parsing date range from URL', e);
      }
    } else {
      dateRange.value = { start: undefined, end: undefined };
    }
  },
  { immediate: true }
);

const buildFilter = (): ArticleQueryInput => {
  let startDate: Date | undefined;
  let endDate: Date | undefined;

  if (dateRange.value.start && dateRange.value.end) {
    startDate = dateRange.value.start.toDate('UTC');
    endDate = dateRange.value.end.toDate('UTC');
    endDate.setHours(23, 59, 59, 999);
  }

  return {
    unread_only: isUnreadOnly.value,
    category_name: '',
    tag_id: '',
    bookmarked_only: isBookmarkedOnly.value,
    related_to_id: '',
    feed_id: effectiveFeedId.value || '',
    start_date: startDate,
    end_date: endDate,
    limit: 30,
    query: localSearchQuery.value.trim(),
  };
};

const backendFilterSignature = computed(() => JSON.stringify({
  unread_only: isUnreadOnly.value,
  bookmarked_only: isBookmarkedOnly.value,
  feed_id: effectiveFeedId.value || '',
  start_date: dateRange.value.start ? dateRange.value.start.toString() : '',
  end_date: dateRange.value.end ? dateRange.value.end.toString() : '',
  query: localSearchQuery.value.trim(),
}));

// On component mount
onMounted(async () => {
  // Kick off feeds fetch (singleton — only fires once across the whole app).
  fetchFeeds();
});

// Virtualize the article list — only mounts ~20 visible rows regardless of total length.
// Item height is an estimate; rows can grow taller for wrapped titles, but the overscan
// hides any minor mismatch.
const ITEM_HEIGHT = 76;
const { list: virtualRows, containerProps, wrapperProps, scrollTo } = useVirtualList(
  currentArticles,
  { itemHeight: ITEM_HEIGHT, overscan: 8 }
);

let filterLoadToken = 0;

watch(
  backendFilterSignature,
  async () => {
    const loadToken = ++filterLoadToken;
    scrollTo(0);

    const articles = await loadInitialArticles(buildFilter());

    if (loadToken !== filterLoadToken) return;

    await nextTick();
    scrollTo(0);

    if (
      route.path.includes('/feed/') &&
      !currentArticleId.value &&
      articles.length > 0
    ) {
      await router.replace({
        path: route.path,
        query: {
          ...route.query,
          articleId: articles[0].id,
        },
      });
    }
  },
  { immediate: true }
);

useInfiniteScroll(
  containerProps.ref,
  async () => {
    await loadMoreArticles();
  },
  {
    distance: 300,
    canLoadMore: () => state.hasMore && !state.loadingMore && !state.loading,
  }
);

</script>

<template>
  <div class="border-r border-gray-800 flex flex-col relative">
    <div class="flex flex-col">
      <SearchBar
        v-model="localSearchQuery"
        placeholder="Search articles..."
        @search="handleSearch"
        :debounce="400"
      />
      <!-- Filter bar -->
      <div class="flex items-center gap-1 px-3 py-1.5 border-b border-gray-800 flex-wrap">
        <!-- Unread / Bookmarked toggles -->
        <Badge
          :variant="isUnreadOnly ? 'default' : 'outline'"
          class="cursor-pointer flex gap-1 items-center text-[10px] font-mono tracking-wide h-6 px-2"
          @click="toggleUnreadFilter"
        >
          <EyeIcon class="h-3 w-3 mr-0.5" />
          UNREAD
        </Badge>

        <Badge
          :variant="isBookmarkedOnly ? 'default' : 'outline'"
          class="cursor-pointer flex gap-1 items-center text-[10px] font-mono tracking-wide h-6 px-2"
          @click="toggleBookmarkFilter"
        >
          <BookmarkIcon class="h-3 w-3 mr-0.5" />
          SAVED
        </Badge>

        <div class="ml-auto" />

        <!-- Date Range Filter -->
        <Popover v-model:open="showDateRangePicker">
          <PopoverTrigger as-child>
            <Button
              :variant="showDateRangePicker ? 'default' : 'outline'"
              class="text-[10px] font-mono h-6 px-2 gap-1 tracking-wide"
              size="sm"
            >
              <CalendarIcon class="h-3 w-3 mr-0.5" />
              <span>{{ formattedDateRange }}</span>
              <ChevronDownIcon class="h-3 w-3 ml-0.5" />
            </Button>
          </PopoverTrigger>
          <PopoverContent class="w-auto p-0" align="start">
            <div class="p-3 border-b">
              <div class="flex items-center justify-between">
                <h4 class="font-medium">Date Range</h4>
                <Button
                  v-if="dateRange.start && dateRange.end"
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2"
                  @click="clearDateRange"
                >
                  Clear
                </Button>
              </div>
            </div>
            <RangeCalendar
              @update:modelValue="updateDateRange"
              class="p-2 rounded-md"
            />
          </PopoverContent>
        </Popover>
      </div>
    </div>

    <div v-if="state.loading && currentArticles.length === 0" class="flex justify-center items-center p-4 text-gray-400">
      Loading articles...
    </div>

    <div v-else-if="currentArticles.length === 0" class="flex justify-center items-center p-4 text-gray-400">
      No articles found
    </div>

    <div v-else v-bind="containerProps" class="flex-grow overflow-auto">
      <div v-bind="wrapperProps">
        <ArticleListItem
          v-for="row in virtualRows"
          :key="row.data.id"
          :article="row.data"
          :isSelected="currentArticleId === row.data.id"
          :articleIndex="row.index"
          :allArticles="currentArticles"
          :lastSelectedArticleId="lastSelectedArticleId"
          :selectedArticleId="currentArticleId"
          @select="selectArticle(row.data)"
        />
      </div>
      <div v-if="state.loadingMore" class="py-3 text-center text-xs text-gray-500">
        Loading more articles...
      </div>
    </div>

    <!-- Floating selection action bar -->
    <div
      v-if="queueStore.selectionCount > 0"
      class="absolute bottom-4 left-1/2 -translate-x-1/2 z-30 flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 text-white shadow-lg"
    >
      <span class="text-sm font-medium">{{ queueStore.selectionCount }} selected</span>
      <Button
        size="sm"
        variant="secondary"
        class="h-7 text-xs gap-1"
        @click="queueStore.enqueueAndStart"
      >
        <SparklesIcon class="h-3.5 w-3.5" />
        Analyze all
      </Button>
      <Button
        size="sm"
        variant="ghost"
        class="h-7 w-7 p-0 text-white hover:text-white hover:bg-blue-700"
        @click="queueStore.clearSelection"
      >
        <XIcon class="h-4 w-4" />
      </Button>
    </div>
  </div>
</template>
