<script setup lang="ts">
import Sidebar from '@/components/layout/Sidebar.vue';
import AnalysisQueue from '@/components/articles/AnalysisQueue.vue';
import { ref, provide, onMounted } from 'vue';
import { ChevronRightIcon, ListOrderedIcon } from 'lucide-vue-next';
import { useAnalysisQueueStore } from '@/stores/analysisQueueStore.ts';

const searchQuery = ref('');
const unreadOnly = ref(false);

// Selected article handling for digest view
const selectedArticleId = ref<string | null>(null);

// Sidebar visibility state - only needed for manual toggling on small screens
const isSidebarVisible = ref(false);

// Queue panel state
const queueStore = useAnalysisQueueStore();
const queueOpen = ref(false);


// Toggle sidebar visibility
const toggleSidebar = () => {
  isSidebarVisible.value = !isSidebarVisible.value;
};

// Provide the selected article to child components
provide('selectedArticleId', selectedArticleId);

// Handle article selection from list
const handleArticleSelected = (articleId: string) => {
  selectedArticleId.value = articleId;
  // Close sidebar when article is selected on small screens
  isSidebarVisible.value = false;
};

// Handle search input - called by child components via search event
const handleSearch = (query: string) => {
  // Update the reactive search query reference
  searchQuery.value = query;
};

// Initialize queue status on mount
onMounted(async () => {
  await queueStore.fetchStatus();
});

</script>

<template>
  <div class="flex h-screen w-full relative">
    <!-- Toggle button - only on small screens when sidebar is collapsed -->
    <div 
      @click="toggleSidebar"
      class="hidden max-lg:flex w-8 items-center justify-center h-full cursor-pointer hover:bg-gray-900 border-r border-gray-700 z-20"
      :class="{ 'hidden': isSidebarVisible }"
      title="Open sidebar"
    >
      <ChevronRightIcon class="h-5 w-5 text-gray-300" />
    </div>

    <!-- Large screen sidebar (part of normal flow) -->
    <div 
      class="hidden lg:block h-full overflow-hidden transition-[width] duration-300 ease-in-out w-[16vw]"
    >
      <div class="h-full bg-background border-r border-gray-700">
        <Sidebar />
      </div>
    </div>

    <!-- Main content wrapper -->
    <div class="flex flex-1 h-full">
      <!-- Router views for nested routes -->
      <router-view 
        name="list"
        :search-query="searchQuery"
        :unread-only="unreadOnly"
        @search="handleSearch"
        @article-selected="handleArticleSelected" 
        class="flex-shrink-0 lg:w-[25vw] w-[35vw]"
      />

      <!-- Main content area -->
      <router-view class="flex-1 lg:w-[35vw] w-[35vw]" />

      <!-- Floating queue button -->
      <button
        @click="queueOpen = true"
        class="fixed bottom-6 right-6 flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white shadow-lg transition-[background-color,box-shadow] duration-200 z-40 group"
        :class="{ 'ring-2 ring-blue-400 shadow-blue-500/30': queueStore.isProcessing }"
        title="Open analysis queue"
      >
        <ListOrderedIcon class="w-5 h-5" />
        <span v-if="queueStore.queueLength > 0" class="text-sm font-medium">{{ queueStore.queueLength }}</span>
        <span v-if="queueStore.isProcessing" class="ml-1 text-xs opacity-75">Processing...</span>
      </button>
    </div>
      
    <!-- Small screen sidebar overlay -->
    <!-- Semi-transparent overlay (mobile only) -->
    <div 
      v-if="isSidebarVisible" 
      @click="toggleSidebar"
      class="hidden max-lg:block fixed inset-0 bg-black/50 z-30"
    ></div>
    
    <!-- Mobile sidebar overlay -->
    <div 
      class="lg:hidden fixed top-0 left-0 h-full overflow-hidden z-40"
      :class="isSidebarVisible ? 'w-[calc(18rem-2rem)]' : 'w-0 invisible'"
    >
      <!-- Close button inside sidebar -->
      <div class="flex h-full">
        <!-- Mobile sidebar content -->
        <div class="h-full w-full bg-background">
          <Sidebar />
        </div>
      </div>
    </div>

    <!-- Analysis Queue Panel -->
    <AnalysisQueue v-model:open="queueOpen" />
  </div>
</template>
