<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAnalysisQueueStore } from '@/stores/analysisQueueStore.ts'
import { useArticles } from '@/composables/useArticles.ts'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetClose } from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { LoaderIcon, Trash2Icon, PlayIcon, StopCircleIcon, CheckCircle2Icon, CircleIcon } from 'lucide-vue-next'

interface Props {
  open: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
}>()

const router = useRouter()
const route = useRoute()
const queueStore = useAnalysisQueueStore()
const { getCachedArticle } = useArticles()
const currentQueuedJobTitle = computed(() => (
  queueStore.currentTitle || queueStore.queue.find(job => job.article_id === queueStore.currentId)?.article_title
))

// Handle clicking on a queued article
const selectQueuedArticle = async (articleId: string) => {
  const query: {
    articleId: string;
    unread?: string;
    bookmarked?: string;
    q?: string;
    feed?: string;
    from?: string;
    to?: string;
  } = {
    articleId: articleId,
    ...(route.query.unread ? { unread: route.query.unread as string } : {}),
    ...(route.query.bookmarked ? { bookmarked: route.query.bookmarked as string } : {}),
    ...(route.query.q ? { q: route.query.q as string } : {}),
    ...(route.query.feed ? { feed: route.query.feed as string } : {}),
    ...(route.query.from ? { from: route.query.from as string } : {}),
    ...(route.query.to ? { to: route.query.to as string } : {}),
  }

  const path = route.query.feed ? `/feed/${route.query.feed}` : '/all'

  await router.push({ path, query })

  // Close the queue panel
  emit('update:open', false)
}

// Prefer queue payload titles; fall back to the article cache for older jobs.
const getArticleTitle = (articleId: string, queuedTitle?: string): string => {
  if (queuedTitle) return queuedTitle
  const article = getCachedArticle(articleId)
  return article?.title ?? 'Unknown Article'
}

// Task display names
const taskLabels: Record<string, string> = {
  categorize: 'Categorize',
  key_points: 'Key Points',
  tags: 'Tags',
  importance: 'Importance Score',
  insights: 'Insights',
  summaries: 'Summaries',
}

// All task names in pipeline order
const allTaskNames = ['categorize', 'key_points', 'tags', 'importance', 'insights', 'summaries']

// Compute task statuses for display
const taskStatuses = computed(() => {
  const progress = queueStore.currentProgress
  const completed = queueStore.completedTasks
  if (!progress) return []

  const completedNames = new Set(completed.map(t => t.task_name))
  const total = progress.total_tasks
  const taskNames = allTaskNames.slice(0, total)

  return taskNames.map(name => {
    let status: 'pending' | 'running' | 'completed' = 'pending'
    if (completedNames.has(name)) {
      status = 'completed'
    } else if (progress.task_name === name && progress.status === 'started') {
      status = 'running'
    }
    return { name, label: taskLabels[name] ?? name, status }
  })
})

const hasProgress = computed(() => taskStatuses.value.length > 0)


</script>

<template>
  <Sheet :open="props.open" @update:open="emit('update:open', $event)">
    <SheetContent side="right" class="flex flex-col gap-0 w-96 p-0">
      <!-- Header -->
      <SheetHeader class="border-b border-gray-700 px-6 py-4">
        <div class="flex items-center justify-between w-full">
          <div class="flex items-center gap-2">
            <SheetTitle class="text-lg">Analysis Queue</SheetTitle>
            <Badge class="ml-2">{{ queueStore.queueLength }}</Badge>
          </div>
          <SheetClose />
        </div>
      </SheetHeader>

      <!-- Content -->
      <ScrollArea class="flex-1">
        <!-- Processing indicator with live task progress -->
        <div v-if="queueStore.isProcessing && queueStore.currentId" class="border-b border-gray-700 px-6 py-4 space-y-3">
          <div class="flex items-center gap-3">
            <LoaderIcon class="w-4 h-4 animate-spin text-blue-500" />
            <div class="flex-1 min-w-0">
              <p class="text-sm font-medium text-gray-300">Analyzing</p>
              <p class="text-xs text-gray-500 truncate">{{ getArticleTitle(queueStore.currentId, currentQueuedJobTitle) }}</p>
            </div>
          </div>

          <!-- Task progress steps -->
          <div v-if="hasProgress" class="ml-1 space-y-1.5">
            <div
              v-for="task in taskStatuses"
              :key="task.name"
              class="flex items-center gap-2.5"
            >
              <!-- Status icon -->
              <CheckCircle2Icon
                v-if="task.status === 'completed'"
                class="w-3.5 h-3.5 text-emerald-500 flex-shrink-0"
              />
              <LoaderIcon
                v-else-if="task.status === 'running'"
                class="w-3.5 h-3.5 text-blue-400 animate-spin flex-shrink-0"
              />
              <CircleIcon
                v-else
                class="w-3.5 h-3.5 text-gray-600 flex-shrink-0"
              />

              <!-- Task label -->
              <span
                class="text-xs"
                :class="{
                  'text-emerald-400': task.status === 'completed',
                  'text-blue-300 font-medium': task.status === 'running',
                  'text-gray-600': task.status === 'pending',
                }"
              >
                {{ task.label }}
              </span>
            </div>

            <!-- Progress bar -->
            <div class="mt-2 h-1 bg-gray-800 rounded-full overflow-hidden">
              <div
                class="h-full bg-blue-500 rounded-full transition-all duration-500 ease-out"
                :style="{ width: `${(queueStore.completedTasks.length / taskStatuses.length) * 100}%` }"
              />
            </div>

          </div>
        </div>

        <!-- Queue list -->
        <div v-if="queueStore.queue.length > 0" class="px-6 py-4 space-y-2">
          <div
            v-for="(job, index) in queueStore.queue"
            :key="job.id"
            class="flex items-center gap-3 p-3 rounded-lg bg-gray-900 hover:bg-gray-800 transition-colors group cursor-pointer"
            :class="{ 'bg-gray-800': queueStore.currentId === job.article_id }"
            @click="selectQueuedArticle(job.article_id)"
          >
            <!-- Position number -->
            <div class="flex-shrink-0 w-6 h-6 rounded-full bg-gray-700 flex items-center justify-center">
              <span class="text-xs font-medium text-gray-300">{{ index + 1 }}</span>
            </div>

            <!-- Article title -->
            <div class="flex-1 min-w-0">
              <p class="text-sm text-gray-300 truncate" :title="getArticleTitle(job.article_id, job.article_title)">
                {{ getArticleTitle(job.article_id, job.article_title) }}
              </p>
            </div>

            <!-- Remove button -->
            <Button
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
              @click.stop="queueStore.dequeueArticle(job.article_id)"
              title="Remove from queue"
            >
              <Trash2Icon class="w-4 h-4 text-red-500" />
            </Button>
          </div>
        </div>

        <!-- Empty state -->
        <div v-else class="flex flex-col items-center justify-center p-8 text-center">
          <p class="text-sm text-gray-400">No articles in queue</p>
          <p class="text-xs text-gray-500 mt-2">Select articles and click "Add to Queue" to get started</p>
        </div>
      </ScrollArea>

      <!-- Footer with actions -->
      <div class="border-t border-gray-700 px-6 py-4 space-y-2">
        <div class="flex gap-2">
          <Button
            v-if="!queueStore.isProcessing"
            size="sm"
            class="flex-1"
            :disabled="queueStore.queue.length === 0"
            @click="queueStore.startQueue"
          >
            <PlayIcon class="w-4 h-4 mr-2" />
            Start Analysis
          </Button>
          <Button
            v-else
            variant="outline"
            size="sm"
            class="flex-1"
            @click="queueStore.stopQueue"
          >
            <StopCircleIcon class="w-4 h-4 mr-2" />
            Stop
          </Button>
        </div>

        <Button
          v-if="queueStore.queue.length > 0"
          variant="ghost"
          size="sm"
          class="w-full text-red-500 hover:text-red-400 hover:bg-red-900/20"
          @click="queueStore.clearQueue"
        >
          <Trash2Icon class="w-4 h-4 mr-2" />
          Clear All
        </Button>
      </div>
    </SheetContent>
  </Sheet>
</template>
