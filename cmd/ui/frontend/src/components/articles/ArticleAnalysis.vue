<script setup lang="ts">
import {LoaderIcon, PencilRulerIcon, Settings2Icon, CheckCircle2Icon, CircleIcon, ChevronDownIcon, ChevronRightIcon, ChevronLeftIcon} from 'lucide-vue-next';
import {computed, nextTick, onMounted, ref, watch} from 'vue';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select';
import {useArticleAnalysisStore} from '@/stores/articleAnalysisStore';
import {useAnalysisQueueStore} from '@/stores/analysisQueueStore';
import {Card} from "@/components/ui/card";

const props = defineProps<{
  articleId: string;
  showAnalysisDialog: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:showAnalysisDialog', value: boolean): void;
}>();

// Use the Pinia stores
const articleAnalysisStore = useArticleAnalysisStore();
const queueStore = useAnalysisQueueStore();
const {
  fetchArticleAnalyses,
  selectAnalysis,
} = articleAnalysisStore;

// Whether this article is currently being analyzed via the queue
const isQueueAnalyzing = computed(() => queueStore.currentId === props.articleId && queueStore.isProcessing)

// Has live log data for this article (persists after analysis completes until user navigates away)
const hasLog = computed(() => {
  const raw = queueStore.analysisLogArticleId === props.articleId ? queueStore.analysisLog : []
  return raw.length > 0
})

// View mode: 'live' shows the streaming progress, 'history' shows a saved analysis
type ViewMode = 'live' | 'history'
const viewMode = ref<ViewMode>(
  (isQueueAnalyzing.value || hasLog.value) ? 'live' : 'history'
)

// Auto-switch to live when a new run starts for this article
watch(isQueueAnalyzing, (analyzing) => {
  if (analyzing) {
    viewMode.value = 'live'
  }
})

watch(() => queueStore.analysisLogArticleId, (id) => {
  if (id === props.articleId && queueStore.isProcessing) {
    viewMode.value = 'live'
  }
})

// Reset viewMode when article changes
watch(() => props.articleId, () => {
  viewMode.value = (isQueueAnalyzing.value || hasLog.value) ? 'live' : 'history'
})

// Handle analysis selection — switches to history view
const handleSelectAnalysis = async (analysisId: string) => {
  viewMode.value = 'history'
  await selectAnalysis(props.articleId, analysisId);
};

const handleGoPrev = async () => {
  viewMode.value = 'history'
  await articleAnalysisStore.goToPrevAnalysis(props.articleId)
}

const handleGoNext = async () => {
  viewMode.value = 'history'
  await articleAnalysisStore.goToNextAnalysis(props.articleId)
}

// Enqueue for default analysis and auto-start
const handleRunDefaultAnalysis = async () => {
  viewMode.value = 'live'
  queueStore.resetProgress(props.articleId);
  await queueStore.enqueueOne(props.articleId);
};

// Log entries from the store — filter out "started" entries once a matching completed/error exists
const logEntries = computed(() => {
  const raw = queueStore.analysisLogArticleId === props.articleId ? queueStore.analysisLog : []
  const finishedTasks = new Set(
    raw.filter(e => e.type === 'completed' || e.type === 'error').map(e => e.taskName)
  )
  return raw.filter(e => e.type !== 'started' || !finishedTasks.has(e.taskName))
})

const expandedEntries = ref<Set<number>>(new Set())
const logContainer = ref<HTMLElement | null>(null)

const toggleEntry = (index: number) => {
  if (expandedEntries.value.has(index)) {
    expandedEntries.value.delete(index)
  } else {
    expandedEntries.value.add(index)
  }
}

// Streaming tokens — only shown for this article while a task is actively streaming
const streamingTokens = computed(() =>
  queueStore.analysisLogArticleId === props.articleId ? queueStore.streamingTokens : ''
)

const streamingEl = ref<HTMLElement | null>(null)

watch(streamingTokens, async () => {
  await nextTick()
  if (streamingEl.value) {
    streamingEl.value.scrollTop = streamingEl.value.scrollHeight
  }
})

const completedCount = computed(() => logEntries.value.filter(e => e.type === 'completed').length)
const totalTasks = computed(() => queueStore.currentProgress?.total_tasks ?? 0)
const isAnalysisComplete = computed(() => queueStore.analysisLogArticleId === props.articleId && queueStore.analysisCompleted)

// Live partial results from the store (accumulates as tasks complete)
const liveResult = computed(() =>
  queueStore.analysisLogArticleId === props.articleId ? queueStore.liveAnalysisResult : null
)

// Auto-scroll and auto-expand completed entries when new log entries arrive
watch(() => logEntries.value.length, (newLen, oldLen) => {
  for (let i = (oldLen ?? 0); i < newLen; i++) {
    if (logEntries.value[i]?.type === 'completed') {
      expandedEntries.value.add(i)
    }
  }
  nextTick(() => {
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  })
})

// Load analyses when the component is mounted
onMounted(async () => {
  await fetchArticleAnalyses(props.articleId);
});

// When the articleId changes, fetch analyses again
watch(() => props.articleId, async (newId) => {
  if (newId) {
    await fetchArticleAnalyses(newId);
  }
});

// Refresh analyses when queue finishes processing this article or when analysis completes
watch(isQueueAnalyzing, async (analyzing, wasAnalyzing) => {
  if (wasAnalyzing && !analyzing) {
    await fetchArticleAnalyses(props.articleId);
  }
});

watch(() => queueStore.analysisCompleted, async (completed) => {
  if (completed && queueStore.analysisLogArticleId === props.articleId) {
    // Delay slightly to allow backend to persist the analysis
    setTimeout(() => {
      fetchArticleAnalyses(props.articleId);
    }, 500);
  }
});
</script>

<template>
  <div>
    <!-- Banner: live run active while viewing history -->
    <div
      v-if="viewMode === 'history' && (isQueueAnalyzing || (hasLog && !isAnalysisComplete))"
      class="mx-4 mt-4 flex items-center gap-2 rounded-lg border border-blue-800 bg-blue-950/40 px-4 py-2 text-sm text-blue-300 cursor-pointer hover:bg-blue-950/60 transition-colors"
      @click="viewMode = 'live'"
    >
      <LoaderIcon class="h-3.5 w-3.5 animate-spin flex-shrink-0"/>
      <span>Analysis is running — click to view live progress</span>
    </div>

    <!-- Live analysis log (queue-based) -->
    <div v-if="viewMode === 'live' && (isQueueAnalyzing || hasLog)" class="my-4 mx-4">
      <!-- Header -->
      <div class="flex items-center gap-3 mb-3">
        <LoaderIcon v-if="isQueueAnalyzing" class="h-4 w-4 animate-spin text-blue-400"/>
        <CheckCircle2Icon v-else class="h-4 w-4 text-emerald-500"/>
        <span class="text-sm font-medium text-gray-300">
          <span v-if="isAnalysisComplete">Analysis Complete</span>
          <span v-else>Analysis Progress</span>
          <span v-if="totalTasks > 0" class="text-gray-500 font-normal ml-1">{{ completedCount }}/{{ totalTasks }}</span>
        </span>
        <!-- Progress bar -->
        <div v-if="totalTasks > 0" class="flex-1 h-1 bg-gray-800 rounded-full overflow-hidden ml-2">
          <div
            class="h-full bg-blue-500 rounded-full transition-all duration-500 ease-out"
            :style="{ width: `${(completedCount / totalTasks) * 100}%` }"
          />
        </div>
      </div>

      <!-- Log scroll area -->
      <div
        ref="logContainer"
        class="bg-gray-950 border border-gray-800 rounded-lg overflow-y-auto max-h-[60vh] font-mono text-xs"
      >
        <div v-if="logEntries.length === 0" class="p-4 text-gray-600 text-center">
          Waiting for first task...
        </div>
        <div v-for="(entry, idx) in logEntries" :key="idx" class="border-b border-gray-800/50 last:border-b-0">
          <!-- Entry header (clickable to toggle) -->
          <div
            @click="toggleEntry(idx)"
            class="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-gray-900/50"
          >
            <ChevronDownIcon v-if="expandedEntries.has(idx)" class="w-3 h-3 text-gray-500 flex-shrink-0"/>
            <ChevronRightIcon v-else class="w-3 h-3 text-gray-500 flex-shrink-0"/>

            <CheckCircle2Icon v-if="entry.type === 'completed'" class="w-3 h-3 text-emerald-500 flex-shrink-0"/>
            <LoaderIcon v-else-if="entry.type === 'started'" class="w-3 h-3 text-blue-400 animate-spin flex-shrink-0"/>
            <CircleIcon v-else class="w-3 h-3 text-red-400 flex-shrink-0"/>

            <span
              class="font-semibold"
              :class="{
                'text-blue-400': entry.type === 'started',
                'text-emerald-400': entry.type === 'completed',
                'text-red-400': entry.type === 'error',
              }"
            >
              {{ entry.label }}
            </span>
            <span class="text-gray-600 ml-1">
              {{ entry.type === 'started' ? 'running...' : entry.type === 'completed' ? 'completed' : 'failed' }}
            </span>
            <span class="text-gray-700 ml-auto">{{ entry.timestamp.toLocaleTimeString() }}</span>
          </div>

          <!-- Streaming tokens (shown while task is running, collapses when done) -->
          <div v-if="entry.type === 'started' && streamingTokens" class="px-3 pb-3">
            <div
              ref="streamingEl"
              class="overflow-y-auto rounded bg-gray-900 px-2.5 py-2 text-xs text-gray-300 font-mono leading-relaxed whitespace-pre-wrap break-words resize-y"
              style="height: 20rem; min-height: 5rem;"
            >{{ streamingTokens }}</div>
          </div>

          <!-- Expanded content (auto-expanded for completed entries) -->
          <div v-if="expandedEntries.has(idx) && entry.content" class="px-3 pb-3">
            <pre class="whitespace-pre-wrap break-words text-gray-400 bg-gray-900 rounded p-3 max-h-80 overflow-y-auto">{{ entry.content }}</pre>
          </div>
        </div>

        <!-- Currently running indicator -->
        <div v-if="isQueueAnalyzing && logEntries.length > 0 && logEntries[logEntries.length - 1].type === 'started'" class="px-3 py-2">
          <div class="flex items-center gap-2 text-gray-600">
            <LoaderIcon class="w-3 h-3 animate-spin"/>
            <span>Waiting for LLM response...</span>
          </div>
        </div>

        <!-- Completion indicator -->
        <div v-if="isAnalysisComplete" class="px-3 py-3 border-t border-gray-800 bg-emerald-950/20">
          <div class="flex items-center gap-2 text-emerald-400">
            <CheckCircle2Icon class="w-4 h-4 flex-shrink-0"/>
            <span class="text-sm font-medium">Analysis complete!</span>
            <span class="text-xs text-gray-600 ml-auto">{{ queueStore.analysisCompletedAt?.toLocaleTimeString() }}</span>
          </div>
        </div>
      </div>

      <div v-if="!isQueueAnalyzing" class="mt-4 flex flex-wrap justify-center gap-2">
        <Button
          @click="handleRunDefaultAnalysis()"
          variant="default"
          size="sm"
          class="bg-emerald-600 hover:bg-emerald-700 text-white flex items-center justify-center gap-2"
          :disabled="queueStore.isArticleQueued(props.articleId)"
        >
          <span class="w-3 h-3 flex items-center justify-center">✨</span>
          Analyze Article
        </Button>

        <Button
          @click="emit('update:showAnalysisDialog', true)"
          variant="outline"
          size="sm"
          class="text-white bg-gray-800 border-gray-700 hover:bg-gray-700 flex items-center justify-center gap-2"
          :disabled="queueStore.isArticleQueued(props.articleId)"
        >
          <Settings2Icon class="w-3 h-3"/>
          Custom Analysis
        </Button>
      </div>

      <!-- Live partial results (appear as tasks complete) -->
      <div v-if="liveResult && Object.keys(liveResult).length > 0" class="mt-6 px-2 max-w-2xl">

        <!-- Importance -->
        <div v-if="liveResult.importance_score !== undefined" class="mb-4">
          <Badge
            class="font-medium px-3 py-1"
            variant="outline"
            :class="{
              'text-red-700 bg-red-100 dark:text-red-300 dark:bg-red-900/30': liveResult.importance_score >= 90,
              'text-amber-700 bg-amber-100 dark:text-amber-300 dark:bg-amber-900/30': liveResult.importance_score > 75 && liveResult.importance_score < 90,
              'text-yellow-700 bg-yellow-100 dark:text-yellow-300 dark:bg-yellow-900/30': liveResult.importance_score > 60 && liveResult.importance_score <= 75,
              'text-gray-700 bg-gray-100 dark:text-gray-300 dark:bg-gray-800': liveResult.importance_score <= 60
            }"
          >
            <span v-if="liveResult.importance_score >= 90">Must Read</span>
            <span v-else-if="liveResult.importance_score >= 75">Should Read</span>
            <span v-else-if="liveResult.importance_score >= 60">May Read</span>
            <span v-else>Optional Reading</span>
            <span class="ml-2 text-xs opacity-80">{{ Math.round(liveResult.importance_score) }}%</span>
          </Badge>
        </div>

        <!-- Category -->
        <div v-if="liveResult.category" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Category</h4>
          <Badge variant="outline">{{ liveResult.category }}</Badge>
        </div>

        <!-- Key Points -->
        <div v-if="liveResult.key_points?.length" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Key Points</h4>
          <ul class="list-disc pl-5 text-gray-200 space-y-1 text-sm">
            <li v-for="(point, i) in liveResult.key_points" :key="i">{{ point }}</li>
          </ul>
        </div>

        <!-- Tags -->
        <div v-if="liveResult.tags?.length" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Tags</h4>
          <div class="flex flex-wrap gap-1">
            <Badge v-for="tag in liveResult.tags" :key="tag" variant="secondary" class="text-xs">{{ tag }}</Badge>
          </div>
        </div>

        <!-- Insights -->
        <div v-if="liveResult.insights?.length" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Insights</h4>
          <ul class="list-disc pl-5 text-gray-200 space-y-1 text-sm">
            <li v-for="(insight, i) in liveResult.insights" :key="i">{{ insight }}</li>
          </ul>
        </div>

        <!-- Justification -->
        <div v-if="liveResult.justification" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Justification</h4>
          <p class="text-gray-200 text-sm">{{ liveResult.justification }}</p>
        </div>

        <!-- Summaries -->
        <div v-if="liveResult.brief_overview" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Brief Overview</h4>
          <p class="text-gray-200 text-sm">{{ liveResult.brief_overview }}</p>
        </div>
        <div v-if="liveResult.standard_synthesis" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Standard Synthesis</h4>
          <p class="text-gray-200 text-sm">{{ liveResult.standard_synthesis }}</p>
        </div>
        <div v-if="liveResult.comprehensive_synthesis" class="mb-4">
          <h4 class="text-sm font-medium text-gray-400 mb-1">Comprehensive Synthesis</h4>
          <p class="text-gray-200 text-sm">{{ liveResult.comprehensive_synthesis }}</p>
        </div>
      </div>

      <!-- Switch to history view once complete and saved analyses are available -->
      <div v-if="isAnalysisComplete && articleAnalysisStore.hasAnalyses" class="mt-4 flex justify-center">
        <Button
          variant="outline"
          size="sm"
          class="text-xs bg-gray-800 border-gray-700 hover:bg-gray-700"
          @click="viewMode = 'history'"
        >
          View saved analysis
        </Button>
      </div>
    </div>

    <!-- Queued but not yet started -->
    <Card v-else-if="queueStore.isArticleQueued(props.articleId)" class="p-6 my-8 shadow-md max-w-sm bg-gray-900 mx-auto flex justify-center items-center">
      <div class="flex flex-col items-center">
        <CircleIcon class="h-6 w-6 mb-3 text-gray-500"/>
        <p class="text-gray-400">Queued for analysis...</p>
      </div>
    </Card>

    <!-- Loading analyses state -->
    <div v-else-if="articleAnalysisStore.isLoadingAnalyses" class="p-8 flex justify-center items-center">
      <div class="flex flex-col items-center">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-gray-400 border-t-white mb-4"></div>
        <p class="text-gray-400">Loading analyses...</p>
      </div>
    </div>

    <!-- No analyses yet state -->
    <div v-else-if="!articleAnalysisStore.hasAnalyses && !isQueueAnalyzing && !queueStore.isArticleQueued(props.articleId)" class="p-8 flex justify-center items-center">
      <div class="flex flex-col items-center text-center max-w-md">
        <PencilRulerIcon class="h-16 w-16 text-gray-600 mb-4"/>
        <h3 class="text-lg font-medium mb-2">No analyses yet</h3>
        <p class="text-gray-400 mb-6">Run an analysis to generate insights about this article</p>
        <div class="flex gap-2">
          <Button
              @click="handleRunDefaultAnalysis()"
              variant="default"
              class="bg-emerald-600 hover:bg-emerald-700 mr-3 text-white flex items-center justify-center gap-2"
              :disabled="isQueueAnalyzing"
          >
            <span class="w-4 h-4 flex items-center justify-center">✨</span>
            Analyze Article
          </Button>

          <Button
              @click="emit('update:showAnalysisDialog', true)"
              variant="outline"
              class="text-white bg-gray-800 border-gray-700 hover:bg-gray-700 flex items-center justify-center gap-2"
          >
            <Settings2Icon class="w-3 h-3 mr-1"/>
            Custom Analysis
          </Button>
        </div>
      </div>
    </div>

    <!-- Historical analysis results -->
    <div v-else class="px-8 max-w-2xl w-full">
      <!-- Analysis header with selector and actions -->
      <div class="mb-6 mt-4">

        <div class="flex justify-between items-center mb-2">
          <label class="text-sm font-medium text-gray-400 block">Select Analysis</label>
          <div class="flex gap-2">
            <Button
                @click="handleRunDefaultAnalysis()"
                variant="outline"
                size="sm"
                class="text-xs bg-gray-800 border-gray-700 hover:bg-gray-700"
            >
              <span class="w-3 h-3 mr-1 flex items-center justify-center">✨</span>
              Analyze Article
            </Button>
            <Button
                @click="emit('update:showAnalysisDialog', true)"
                variant="outline"
                size="sm"
                class="text-xs bg-gray-800 border-gray-700 hover:bg-gray-700"
            >
              <Settings2Icon class="w-3 h-3 mr-1"/>
              Custom Analysis
            </Button>
          </div>
        </div>

        <!-- Run navigation: prev/next + dropdown -->
        <div class="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            :disabled="!articleAnalysisStore.canGoPrev"
            @click="handleGoPrev"
            class="h-8 w-8 flex-shrink-0 bg-gray-800 border-gray-700 hover:bg-gray-700 disabled:opacity-40"
          >
            <ChevronLeftIcon class="h-4 w-4"/>
          </Button>

          <Select
            v-model="articleAnalysisStore.selectedAnalysisId"
            @update:modelValue="handleSelectAnalysis($event as string)"
            class="flex-1"
          >
            <SelectTrigger class="bg-gray-800 border-gray-700">
              <SelectValue placeholder="Select an analysis"/>
            </SelectTrigger>
            <SelectContent class="select-content-fix">
              <SelectItem
                  v-for="analysis in articleAnalysisStore.allAnalyses"
                  :key="analysis.id"
                  :value="analysis.id"
              >
                <div class="flex select-item-fix items-center gap-4">
                  <span>
                    {{ analysis.provider_type }} / {{ analysis.model_name }}
                  </span>
                  <div class="mx-auto"></div>
                  <span class="text-xs text-gray-500">
                    {{ new Date(String(analysis.created_at)).toLocaleString() }}
                  </span>
                </div>
              </SelectItem>
            </SelectContent>
          </Select>

          <Button
            variant="outline"
            size="icon"
            :disabled="!articleAnalysisStore.canGoNext"
            @click="handleGoNext"
            class="h-8 w-8 flex-shrink-0 bg-gray-800 border-gray-700 hover:bg-gray-700 disabled:opacity-40"
          >
            <ChevronRightIcon class="h-4 w-4"/>
          </Button>
        </div>

        <!-- Position indicator -->
        <div v-if="articleAnalysisStore.allAnalyses.length > 1" class="mt-1 text-center text-xs text-gray-600">
          {{ articleAnalysisStore.currentIndex + 1 }} of {{ articleAnalysisStore.allAnalyses.length }}
          <span class="ml-1">(newest first)</span>
        </div>
      </div>

      <!-- Analysis content -->
      <div v-if="articleAnalysisStore.analysisResult" class="mt-4">

        <!-- Importance -->
        <div v-if="articleAnalysisStore.analysisResult.importance_score !== undefined" class="mb-6">
          <div>
            <Badge
                class="font-medium px-3 py-1"
                variant="outline"
                :class="{
                'text-red-700 bg-red-100 dark:text-red-300 dark:bg-red-900/30': articleAnalysisStore.analysisResult.importance_score >= 90,
                'text-amber-700 bg-amber-100 dark:text-amber-300 dark:bg-amber-900/30': articleAnalysisStore.analysisResult.importance_score > 75 && articleAnalysisStore.analysisResult.importance_score < 90,
                'text-yellow-700 bg-yellow-100 dark:text-yellow-300 dark:bg-yellow-900/30': articleAnalysisStore.analysisResult.importance_score > 60 && articleAnalysisStore.analysisResult.importance_score <= 75,
                'text-gray-700 bg-gray-100 dark:text-gray-300 dark:bg-gray-800': articleAnalysisStore.analysisResult.importance_score <= 60
              }"
            >
              <span v-if="articleAnalysisStore.analysisResult.importance_score >= 90">Must Read</span>
              <span v-else-if="articleAnalysisStore.analysisResult.importance_score >= 75">Should Read</span>
              <span v-else-if="articleAnalysisStore.analysisResult.importance_score >= 60">May Read</span>
              <span v-else>Optional Reading</span>
              <span class="ml-2 text-xs opacity-80">{{
                  Math.round(articleAnalysisStore.analysisResult.importance_score)
                }}%</span>
            </Badge>
          </div>
        </div>

        <!-- Key Points -->
        <div
            v-if="articleAnalysisStore.analysisResult.key_points && articleAnalysisStore.analysisResult.key_points.length > 0"
            class="mb-6">
          <h4 class="text-lg font-medium text-white mb-2">Key Points</h4>
          <ul class="list-disc pl-5 text-gray-200 space-y-1">
            <li v-for="(point, index) in articleAnalysisStore.analysisResult.key_points" :key="index">{{ point }}</li>
          </ul>
        </div>

        <!-- Insights -->
        <div
            v-if="articleAnalysisStore.analysisResult.insights && articleAnalysisStore.analysisResult.insights.length > 0"
            class="mb-6">
          <h4 class="text-lg font-medium text-white mb-2">Insights</h4>
          <ul class="list-disc pl-5 text-gray-200 space-y-1">
            <li v-for="(insight, index) in articleAnalysisStore.analysisResult.insights" :key="index">{{ insight }}</li>
          </ul>
        </div>

        <!-- Justification -->
        <div v-if="articleAnalysisStore.analysisResult.justification" class="mb-6">
          <h4 class="text-lg font-medium text-white mb-2">Justification</h4>
          <p class="text-gray-200">{{ articleAnalysisStore.analysisResult.justification }}</p>
        </div>

        <!-- Summaries Section -->
        <div
            v-if="articleAnalysisStore.analysisResult.brief_overview || articleAnalysisStore.analysisResult.standard_synthesis || articleAnalysisStore.analysisResult.comprehensive_synthesis"
            class="mb-6">
          <h4 class="text-lg font-medium text-white mb-2">Summaries</h4>

          <!-- Brief Overview -->
          <div v-if="articleAnalysisStore.analysisResult.brief_overview" class="mb-4">
            <h5 class="text-md font-medium text-white mb-2">Brief Overview</h5>
            <p class="text-gray-200">{{ articleAnalysisStore.analysisResult.brief_overview }}</p>
          </div>

          <!-- Standard Synthesis -->
          <div v-if="articleAnalysisStore.analysisResult.standard_synthesis" class="mb-4">
            <h5 class="text-md font-medium text-white mb-2">Standard Synthesis</h5>
            <p class="text-gray-200">{{ articleAnalysisStore.analysisResult.standard_synthesis }}</p>
          </div>

          <!-- Comprehensive Synthesis -->
          <div v-if="articleAnalysisStore.analysisResult.comprehensive_synthesis" class="mb-4">
            <h5 class="text-md font-medium text-white mb-2">Comprehensive Synthesis</h5>
            <p class="text-gray-200">{{ articleAnalysisStore.analysisResult.comprehensive_synthesis }}</p>
          </div>
        </div>

        <!-- Thinking Process -->
        <div v-if="articleAnalysisStore.analysisResult.thinking_process" class="mb-6">
          <h4 class="text-lg font-medium text-white mb-2">Thinking Process</h4>
          <p class="text-gray-200">{{ articleAnalysisStore.analysisResult.thinking_process }}</p>
        </div>

      </div>
    </div>
  </div>
</template>
