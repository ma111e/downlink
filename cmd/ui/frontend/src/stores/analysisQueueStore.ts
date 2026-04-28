import { defineStore } from 'pinia'
import { computed, ref, triggerRef } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  EnqueueArticles,
  DequeueArticle,
  GetQueueStatus,
  ClearQueue,
  StartQueue,
  StopQueue
} from '../../wailsjs/go/downlinkclient/DownlinkClient'
import { useArticles } from '@/composables/useArticles'

export interface QueueJob {
  id: string
  article_id: string
  article_title?: string
  provider_type?: string
  model_name?: string
}

export interface QueueStatus {
  queue: QueueJob[]
  current_id: string
  current_title?: string
  is_processing: boolean
}

export interface AnalysisProgress {
  article_id: string
  task_name: string
  status: string // "started" | "token" | "completed" | "error" | "done"
  task_index: number
  total_tasks: number
  task_result?: string
  token_chunk?: string
  error?: string
}

export interface AnalysisLogEntry {
  type: 'started' | 'completed' | 'error'
  taskName: string
  label: string
  content: string
  timestamp: Date
}

export const useAnalysisQueueStore = defineStore('analysisQueue', () => {
  // Queue state (mirrors Go QueueStatus)
  const queue = ref<QueueJob[]>([])
  const currentId = ref<string>('')
  const currentTitle = ref<string>('')
  const isProcessing = ref(false)

  // Live analysis progress for the current job
  const currentProgress = ref<AnalysisProgress | null>(null)
  const completedTasks = ref<AnalysisProgress[]>([])

  // Persistent log entries for the current analysis (survives component unmount/remount)
  const analysisLog = ref<AnalysisLogEntry[]>([])
  const analysisLogArticleId = ref<string>('')

  // Track whether the current analysis completed successfully
  const analysisCompleted = ref(false)
  const analysisCompletedAt = ref<Date | null>(null)

  // Live partial analysis result — accumulates fields as tasks complete
  const liveAnalysisResult = ref<Record<string, any>>({})

  // Accumulated token chunks for the currently-streaming task
  // _tokenBuffer accumulates raw tokens; streamingTokens is throttled to 1 update/s
  let _tokenBuffer = ''
  const streamingTokens = ref<string>('')
  let _tokenFlushTimer: ReturnType<typeof setInterval> | null = null

  const _startTokenFlush = () => {
    if (_tokenFlushTimer !== null) return
    _tokenFlushTimer = setInterval(() => {
      if (streamingTokens.value !== _tokenBuffer) {
        streamingTokens.value = _tokenBuffer
      }
    }, 1000)
  }

  const _stopTokenFlush = () => {
    if (_tokenFlushTimer !== null) {
      clearInterval(_tokenFlushTimer)
      _tokenFlushTimer = null
    }
  }

  // Task display names
  const taskLabels: Record<string, string> = {
    categorize: 'Category & Tags',
    key_points: 'Key Points',
    importance: 'Importance Score',
    insights: 'Insights',
    summaries: 'Summaries',
  }

  // Multi-select UI state (frontend only)
  const selectedArticleIds = ref<Set<string>>(new Set())

  // Computed properties
  const queueLength = computed(() => queue.value.length)
  const selectionCount = computed(() => selectedArticleIds.value.size)

  const isArticleQueued = (id: string): boolean => {
    return queue.value.some(job => job.article_id === id)
  }

  const isArticleSelected = (id: string): boolean => {
    return selectedArticleIds.value.has(id)
  }

  // Sync from Wails event
  const handleQueueUpdate = (status: QueueStatus) => {
    const prevCurrentId = currentId.value

    queue.value = status.queue || []
    currentId.value = status.current_id || ''
    currentTitle.value = status.current_title || ''
    isProcessing.value = status.is_processing || false

    // When an article finishes analysis (currentId transitions away from a value),
    // refresh the articles list so tags/category updates are reflected.
    if (prevCurrentId && prevCurrentId !== currentId.value) {
      const { refreshCurrentList } = useArticles()
      refreshCurrentList()
    }
  }

  // Handle analysis progress events from the streaming RPC
  const handleAnalysisProgress = (progress: AnalysisProgress) => {
    // Reset log and live result if a new article starts (but preserve if previous completed)
    if (progress.article_id && progress.article_id !== analysisLogArticleId.value) {
      analysisLog.value = []
      analysisLogArticleId.value = progress.article_id
      liveAnalysisResult.value = {}
      analysisCompleted.value = false
      analysisCompletedAt.value = null
    }

    const label = taskLabels[progress.task_name] ?? progress.task_name

    if (progress.status === 'started') {
      _tokenBuffer = ''
      streamingTokens.value = ''
      _startTokenFlush()
      currentProgress.value = progress
      analysisLog.value.push({
        type: 'started',
        taskName: progress.task_name,
        label,
        content: progress.task_result ?? '',
        timestamp: new Date(),
      })
    } else if (progress.status === 'token') {
      _tokenBuffer += progress.token_chunk ?? ''
    } else if (progress.status === 'completed') {
      _stopTokenFlush()
      _tokenBuffer = ''
      streamingTokens.value = ''
      completedTasks.value.push(progress)
      currentProgress.value = progress
      analysisLog.value.push({
        type: 'completed',
        taskName: progress.task_name,
        label,
        content: progress.task_result ?? '',
        timestamp: new Date(),
      })
      // Merge parsed task result into live analysis
      if (progress.task_result) {
        try {
          const parsed = JSON.parse(progress.task_result)
          liveAnalysisResult.value = { ...liveAnalysisResult.value, ...parsed }
        } catch { /* ignore parse errors */ }
      }
    } else if (progress.status === 'error') {
      analysisLog.value.push({
        type: 'error',
        taskName: progress.task_name,
        label,
        content: progress.error ?? 'Unknown error',
        timestamp: new Date(),
      })
      // Only reset state for fatal errors (no task_name = pipeline-level error)
      if (!progress.task_name) {
        currentProgress.value = null
        completedTasks.value = []
      }
    } else if (progress.status === 'done') {
      _stopTokenFlush()
      _tokenBuffer = ''
      streamingTokens.value = ''
      currentProgress.value = null
      completedTasks.value = []
      analysisCompleted.value = true
      analysisCompletedAt.value = new Date()
    }
  }

  // Register Wails event listeners on store initialization
  EventsOn('queue:update', handleQueueUpdate)
  EventsOn('analysis:progress', handleAnalysisProgress)

  // Fetch initial queue status from the backend
  const fetchStatus = async () => {
    try {
      const status = await GetQueueStatus()
      handleQueueUpdate(status)
    } catch (error) {
      console.error('Failed to fetch queue status:', error)
    }
  }

  // Multi-select actions — use triggerRef after Set mutations to ensure reactivity
  const toggleSelection = (id: string) => {
    if (selectedArticleIds.value.has(id)) {
      selectedArticleIds.value.delete(id)
    } else {
      selectedArticleIds.value.add(id)
    }
    triggerRef(selectedArticleIds)
  }

  const selectRange = (startId: string, endId: string, allArticleIds: string[]) => {
    const startIndex = allArticleIds.indexOf(startId)
    const endIndex = allArticleIds.indexOf(endId)

    if (startIndex === -1 || endIndex === -1) return

    const [minIndex, maxIndex] = startIndex <= endIndex
      ? [startIndex, endIndex]
      : [endIndex, startIndex]

    for (let i = minIndex; i <= maxIndex; i++) {
      selectedArticleIds.value.add(allArticleIds[i])
    }
    triggerRef(selectedArticleIds)
  }

  const selectAll = (ids: string[]) => {
    selectedArticleIds.value = new Set(ids)
  }

  const clearSelection = () => {
    selectedArticleIds.value = new Set()
  }

  // Enqueue selected articles (queue auto-starts if idle)
  const enqueueAndStart = async () => {
    const ids = Array.from(selectedArticleIds.value)
    if (ids.length === 0) return

    try {
      await EnqueueArticles({ article_ids: ids })
      clearSelection()
    } catch (error) {
      console.error('Failed to enqueue articles:', error)
    }
  }

  // Reset all progress state for the current analysis
  const resetProgress = (articleId?: string) => {
    analysisLog.value = []
    analysisLogArticleId.value = articleId ?? ''
    liveAnalysisResult.value = {}
    analysisCompleted.value = false
    analysisCompletedAt.value = null
    currentProgress.value = null
    completedTasks.value = []
    _stopTokenFlush()
    _tokenBuffer = ''
    streamingTokens.value = ''
  }

  // Enqueue a single article with optional provider/model (queue auto-starts if idle)
  const enqueueOne = async (articleId: string, providerType = '', modelName = '') => {
    try {
      await EnqueueArticles({
        article_ids: [articleId],
        provider_type: providerType,
        model_name: modelName
      })
    } catch (error) {
      console.error('Failed to enqueue article:', error)
    }
  }

  const dequeueArticle = async (articleId: string) => {
    try {
      await DequeueArticle(articleId)
    } catch (error) {
      console.error('Failed to dequeue article:', error)
    }
  }

  const clearQueue = async () => {
    try {
      await ClearQueue()
    } catch (error) {
      console.error('Failed to clear queue:', error)
    }
  }

  const startQueue = async () => {
    try {
      await StartQueue()
    } catch (error) {
      console.error('Failed to start queue:', error)
    }
  }

  const stopQueue = async () => {
    try {
      await StopQueue()
    } catch (error) {
      console.error('Failed to stop queue:', error)
    }
  }

  return {
    // State
    queue,
    currentId,
    currentTitle,
    isProcessing,
    selectedArticleIds,
    currentProgress,
    completedTasks,
    analysisLog,
    analysisLogArticleId,
    analysisCompleted,
    analysisCompletedAt,
    liveAnalysisResult,
    streamingTokens,
    taskLabels,

    // Computed
    queueLength,
    selectionCount,
    isArticleQueued,
    isArticleSelected,

    // Actions
    fetchStatus,
    toggleSelection,
    selectRange,
    selectAll,
    clearSelection,
    enqueueAndStart,
    enqueueOne,
    resetProgress,
    dequeueArticle,
    clearQueue,
    startQueue,
    stopQueue
  }
})
