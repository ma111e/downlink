<script setup lang="ts">
import { PencilRulerIcon, FileTextIcon } from 'lucide-vue-next';
import { onMounted, ref, watch, onBeforeUnmount } from 'vue';
import { useRoute } from 'vue-router';
import { useAnalysisQueueStore } from '@/stores/analysisQueueStore';
import { models } from "../../../wailsjs/go/models.ts";
import Article = models.Article;
import { useFeeds } from '@/composables/useFeeds.ts';
import { useArticles } from '@/composables/useArticles.ts';
import { useArticleAnalysisStore } from '@/stores/articleAnalysisStore';
import { useToast } from "@/components/ui/toast";
import SelectionWheel from '@/components/SelectionWheel.vue';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

// Import extracted components
import ArticleHeader from '@/components/articles/ArticleHeader.vue';
import ArticleActions from '@/components/articles/ArticleActions.vue';
import ArticleContent from '@/components/articles/ArticleContent.vue';
import ArticleAnalysis from '@/components/articles/ArticleAnalysis.vue';
import ArticleTags from '@/components/articles/ArticleTags.vue';
import AnalysisDialog from '@/components/articles/AnalysisDialog.vue';
import AnalysisIndicator from '@/components/articles/AnalysisIndicator.vue';
import { BrowserOpenURL } from "../../../wailsjs/runtime";

const props = defineProps<{
  articleId?: string;
}>();

const route = useRoute();

const article = ref<Article | null>(null);
const feedTitle = ref<string>('');
const { getFeedTitle } = useFeeds();
const { getArticle, updateArticle } = useArticles();
const { toast } = useToast();

// Use the Pinia store for article analysis
const articleAnalysisStore = useArticleAnalysisStore();
const queueStore = useAnalysisQueueStore();

// UI state
const activeTab = ref('content');
const showAnalysisDialog = ref(false);

// Selection wheel state
const wheelVisible = ref(false);
const wheelPosition = ref({ x: 0, y: 0 });

// Update feed title when article changes
const updateFeedTitle = async () => {
  if (article.value?.feed_id) {
    feedTitle.value = await getFeedTitle(article.value.feed_id);
  }
};

// Function to reset article view state
const resetArticleView = () => {
  article.value = null;
  feedTitle.value = '';
  activeTab.value = 'content';
  showAnalysisDialog.value = false;
};

// Function to fetch article data
const fetchArticleData = async (explicitId?: string) => {
  const id = explicitId || props.articleId || route.query.articleId?.toString();
  
  if (!id) {
    resetArticleView();
    return;
  }
  
  try {
    const fetchedArticle = await getArticle(id);
    if (fetchedArticle) {
      article.value = fetchedArticle;
      await updateFeedTitle();
      await articleAnalysisStore.fetchArticleAnalyses(id);


    } else {
      resetArticleView();
      toast({
        title: "Article not found",
        description: "The requested article could not be found",
        variant: "destructive",
        duration: 3000
      });
    }
  } catch (error) {
    console.error("Error fetching article:", error);
    resetArticleView();
    toast({
      title: "Error",
      description: "Could not load the article",
      variant: "destructive",
      duration: 3000
    });
  }
};

// Article updated handler
const onArticleUpdated = (updatedArticle: models.Article) => {
  article.value = updatedArticle;
};

// Analysis handlers
const onAnalysisStarted = () => {
  // Switch to the analysis tab
  activeTab.value = 'analysis';
};

const onAnalysisFinished = async (analysis: any) => {
  if (analysis) {
    // Refetch the article to get updated tags from the analysis
    if (article.value?.id) {
      await fetchArticleData(article.value.id);
    }
    // Refresh the analysis tab to show new analysis
    activeTab.value = 'analysis';
  }
};

// When queue finishes analyzing the currently viewed article, re-fetch it to show updated tags/analysis
watch(() => queueStore.currentId, async (newId, oldId) => {
  if (oldId && oldId === article.value?.id && newId !== oldId) {
    await fetchArticleData(article.value.id);
    await articleAnalysisStore.fetchArticleAnalyses(article.value.id);
  }
});

// Watch for changes in article Id from props
watch(() => props.articleId, async (newId, oldId) => {
  console.log(`ArticleDetail: articleId changed from ${oldId} to ${newId}`);
  if (newId) {
    await fetchArticleData(newId);
  } else {
    resetArticleView();
  }
});

// Selection wheel functions
const showSelectionWheel = (event: MouseEvent) => {
  // Only show on middle mouse button
  if (event.button !== 1) return;
  
  event.preventDefault();
  wheelPosition.value = { x: event.clientX, y: event.clientY };
  wheelVisible.value = true;
};

const onWheelAction = (action: string) => {
  if (!article.value) return;
  
  switch (action) {
    case 'open':
      // Open in web browser
      if (article.value.link) {
        BrowserOpenURL(article.value.link);
      }
      break;
    case 'read':
      // Toggle read status
      updateArticle(article.value.id, { read: !article.value.read }).then((updatedArticle) => {
        if (updatedArticle) {
          article.value = updatedArticle;
        }
      });
      break;
    case 'bookmark':
      // Toggle bookmark
      updateArticle(article.value.id, { bookmarked: !article.value.bookmarked }).then((updatedArticle) => {
        if (updatedArticle) {
          article.value = updatedArticle;
        }
      });
      break;
    case 'share':
      // Share article
      if (article.value.link) {
        navigator.clipboard.writeText(article.value.link);
        toast({
          title: "Link copied to clipboard",
          description: "You can now share the link",
          duration: 3000,
        });
      }
      break;
      
    case 'analyze':
      // Send to analysis queue
      if (article.value.id) {
        activeTab.value = 'analysis';
        queueStore.enqueueOne(article.value.id);
        toast({
          title: "Queued for analysis",
          description: "Your article has been added to the analysis queue",
          duration: 3000,
        });
      }
      break;

    case 'custom':
      // Show custom analysis dialog
      if (article.value.id) {
        activeTab.value = 'analysis';
        showAnalysisDialog.value = true;
      }
      break;
  }
  
  wheelVisible.value = false;
};

const onWheelClose = () => {
  wheelVisible.value = false;
};

// Keyboard shortcuts handler
const handleKeyPress = (event: KeyboardEvent) => {
  if (!article.value) return;
  
  // Prevent handling if in an input field
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes((event.target as HTMLElement).tagName)) {
    return;
  }
  
  switch (event.key) {
    case 'o':
      // Open in web browser
      if (article.value.link) {
        BrowserOpenURL(article.value.link);
      }
      break;
    case 'r':
      // Toggle read status
      updateArticle(article.value.id, { read: !article.value.read }).then((updatedArticle) => {
        if (updatedArticle) {
          article.value = updatedArticle;
        }
      });
      break;
    case 'b':
      // Toggle bookmark
      updateArticle(article.value.id, { bookmarked: !article.value.bookmarked }).then((updatedArticle) => {
        if (updatedArticle) {
          article.value = updatedArticle;
        }
      });
      break;
    case 'a':
      // Show analysis tab
      activeTab.value = 'analysis';
      break;
    case 'c':
      // Show content tab
      activeTab.value = 'content';
      break;
    case '?':
      // Show keyboard shortcuts
      toast({
        title: "Keyboard Shortcuts",
        description: "O - Open in browser\nR - Toggle read status\nB - Toggle bookmark\nA - Show analysis\nC - Show content\n? - Show this help",
        duration: 5000,
      });
      break;
  }
};

// Lifecycle hooks for responsive functionality
onMounted(async () => {
  // Check for article Id in both props and query parameters
  const articleId = props.articleId || route.query.articleId?.toString();

  // Then fetch article data if Id is provided
  if (articleId) {
    await fetchArticleData(articleId);
  } else {
    resetArticleView();
  }
  
  // Add event listeners
  document.addEventListener('keydown', handleKeyPress);
});

onBeforeUnmount(() => {
  // Remove event listeners
  document.removeEventListener('keydown', handleKeyPress);
});
</script>

<template>
  <div class="flex flex-1 flex-col bg-gray-950" @mousedown.middle="showSelectionWheel">
    <!-- <div v-if="state.loading" class="flex justify-center items-center h-full">
      <p>Loading article...</p>
    </div> -->

    <div v-if="!article" class="flex justify-center items-center h-full text-gray-400 flex-col">
      <p class="text-xl mb-4">Select an article to view its content</p>
    </div>

    <div v-else-if="article" class="flex flex-col h-full w-full mx-auto">
      <!-- Article header -->
      <ArticleHeader :article="article" :feed-title="feedTitle" />

      <!-- Article actions and tags -->
      <div class="bg-gray-900 p-2 flex justify-between items-center">
        <ArticleTags :article="article" />
        <ArticleActions :article="article" @article-updated="onArticleUpdated" />
      </div>

      <!-- Content and Analysis Tabs -->
      <div class="flex flex-col bg-gray-950 flex-1 px-8 overflow-auto">
        <Tabs v-model="activeTab" class="w-full max-w-2xl mx-auto" activator="data-state">
          <TabsList class="grid w-full grid-cols-2 mt-4 bg-gray-800">
            <TabsTrigger value="content" class="gap-2">
              <div class="flex items-center justify-center py-3">
                <FileTextIcon class="h-4 w-4 mr-2" />
                Article
              </div>
            </TabsTrigger>
            <TabsTrigger value="analysis" class="shadow-sm flex items-center justify-center gap-2">
              <div class="flex items-center justify-center py-3">
                <AnalysisIndicator class="mr-2" v-if="article" :article-id="article.id" />
                <PencilRulerIcon v-else class="h-4 w-4 mr-2" />
                Analysis
                <span v-if="articleAnalysisStore.allAnalyses.length > 0" class="ml-2">
                  ({{ articleAnalysisStore.allAnalyses.length }})
                </span>
              </div>
            </TabsTrigger>
          </TabsList>
          
          <!-- Content Tab -->
          <TabsContent value="content" class="border-none p-0">
            <ArticleContent v-if="article" :article="article" />
          </TabsContent>
          
          <!-- Analysis Tab -->
          <TabsContent value="analysis" class="border-none p-0">
            <ArticleAnalysis 
              v-if="article" 
              :article-id="article.id" 
              :show-analysis-dialog="showAnalysisDialog"
              @update:show-analysis-dialog="showAnalysisDialog = $event"
            />
          </TabsContent>
        </Tabs>
      </div>
    </div>

    <!-- Analysis Dialog -->
    <AnalysisDialog 
      v-if="article"
      :article-id="article.id"
      :open="showAnalysisDialog"
      @update:open="showAnalysisDialog = $event"
      @analysis-started="onAnalysisStarted"
      @analysis-finished="onAnalysisFinished"
    />

    <!-- Selection Wheel -->
    <SelectionWheel
      v-if="wheelVisible"
      :position="wheelPosition"
      :actions="{
        open: 'Open in Browser',
        read: article?.read ? 'Mark as Unread' : 'Mark as Read',
        bookmark: article?.bookmarked ? 'Remove Bookmark' : 'Add Bookmark',
        share: 'Share',
        analyze: 'Analyze Article',
        custom: 'Custom Analysis'
      }"
      @action="onWheelAction"
      @close="onWheelClose"
    />
  </div>
</template>
