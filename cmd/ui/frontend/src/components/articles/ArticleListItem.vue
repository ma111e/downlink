<script setup lang="ts">
import {BookmarkIcon, ListOrderedIcon} from 'lucide-vue-next';
import {useFormatters} from '@/composables/useFormatters.ts';
import {useFeeds} from '@/composables/useFeeds.ts';
import {models} from "../../../wailsjs/go/models.ts";
import {computed} from 'vue';
import AnalysisIndicator from '@/components/articles/AnalysisIndicator.vue';
import {useAnalysisQueueStore} from '@/stores/analysisQueueStore.ts';
import Article = models.Article;

const SOURCE_COLORS: Record<string, string> = {
  "therecord.media":            "var(--src-therecord)",
  "bleepingcomputer.com":       "var(--src-bleeping)",
  "blog.talosintelligence.com": "var(--src-talos)",
  "wired.com":                  "var(--src-wired)",
};
function sourceColor(url: string): string {
  try {
    const host = new URL(url.startsWith('http') ? url : `https://${url}`).hostname.replace(/^www\./, '');
    return SOURCE_COLORS[host] || "var(--src-default)";
  } catch {
    return SOURCE_COLORS[url] || "var(--src-default)";
  }
}

const props = defineProps<{
  article: Article;
  isSelected: boolean;
  articleIndex?: number;
  allArticles?: Article[];
  lastSelectedArticleId?: string | null;
  selectedArticleId?: string | null;
}>();

const emit = defineEmits<{
  (e: 'select'): void;
}>();

const {timeAgo} = useFormatters();
const {getFeedTitleSync} = useFeeds();
const queueStore = useAnalysisQueueStore();

// Feed title is read synchronously from the cached singleton (populated by ArticleList on mount).
const feedTitle = computed(() => getFeedTitleSync(props.article.feed_id));

// Importance score now comes from the article payload itself — no per-item IPC call.
const importanceScore = computed(() => {
  const s = props.article.latest_importance_score;
  return s != null ? Number(s) : null;
});

// Handle click — ctrl/cmd+click toggles, shift+click selects range, normal click selects article
const handleClick = (event: MouseEvent) => {
  if (event.ctrlKey || event.metaKey) {
    // Ctrl+Click (or Cmd+Click on Mac) — toggle individual selection
    // If this is the first ctrl+click, also add the currently viewed article
    if (props.selectedArticleId && queueStore.selectionCount === 0) {
      queueStore.toggleSelection(props.selectedArticleId)
    }
    queueStore.toggleSelection(props.article.id)
  } else if (event.shiftKey) {
    // Shift+Click — select range from last selected to current
    if (props.lastSelectedArticleId && props.allArticles) {
      const allArticleIds = props.allArticles.map(a => a.id)
      queueStore.selectRange(props.lastSelectedArticleId, props.article.id, allArticleIds)
    }
  } else {
    // Normal click — select and view article
    queueStore.clearSelection()
    emit('select')
  }
};

const importanceLabel = computed(() => {
  const score = importanceScore.value;
  if (score == null) return null;
  if (score >= 90) return 'MUST';
  if (score >= 75) return 'SHOULD';
  if (score >= 60) return 'MAY';
  return null;
});

const importancePriorityClass = computed(() => {
  const score = importanceScore.value;
  if (score == null) return '';
  if (score >= 90) return 'priority-must';
  if (score >= 75) return 'priority-should';
  if (score >= 60) return 'priority-may';
  return '';
});

const importanceCssColor = computed(() => {
  const score = importanceScore.value;
  if (score == null) return 'var(--muted-foreground)';
  if (score >= 90) return 'var(--priority-must)';
  if (score >= 75) return 'var(--priority-should)';
  if (score >= 60) return 'var(--priority-may)';
  return 'var(--muted-foreground)';
});

const feedSourceColor = computed(() => sourceColor(feedTitle.value || ''));
</script>

<template>
  <div
      @click="handleClick($event)"
      class="article-row border-b hover:bg-gray-800/60 cursor-pointer relative group transition-colors duration-100"
      :class="[
        importancePriorityClass,
        {
          'bg-gray-800/80': isSelected,
          'bg-blue-900/20': queueStore.isArticleSelected(props.article.id) && !isSelected,
          'opacity-70': props.article.read && !isSelected && !queueStore.isArticleSelected(props.article.id)
        }
      ]"
  >
    <!-- Hero image faint bleed -->
    <div
        v-if="props.article.hero_image"
        class="absolute inset-0 overflow-hidden pointer-events-none opacity-10 group-hover:opacity-20 -z-1 transition-opacity"
    >
      <div
          class="absolute right-0 top-0 bottom-0 w-2/3 bg-cover bg-center bg-no-repeat"
          :style="{
            backgroundImage: `url(${props.article.hero_image})`,
            maskImage: 'linear-gradient(to right, transparent 0%, black 40%)'
          }"
      ></div>
    </div>

    <div class="px-4 pt-3 pb-2 pl-5">
      <!-- Title row -->
      <div class="flex items-start gap-2 pr-16">
        <h3
          class="text-sm leading-snug flex-1"
          :class="props.article.read ? 'text-gray-400 font-normal' : 'text-gray-100 font-medium'"
        >{{ props.article.title }}</h3>
      </div>

      <!-- Meta row -->
      <div class="flex items-center gap-2 mt-1.5 flex-wrap">
        <!-- Source dot + name -->
        <span
          class="w-1.5 h-1.5 rounded-full flex-shrink-0 inline-block"
          :style="{ background: feedSourceColor }"
        />
        <span
          class="font-mono text-[10px] tracking-wide"
          :style="{ color: feedSourceColor }"
        >{{ feedTitle }}</span>

        <span class="text-gray-600 font-mono text-[10px]">·</span>
        <span class="font-mono text-[10px] text-gray-500">{{ timeAgo(props.article.published_at) }}</span>

        <!-- Priority badge -->
        <span
          v-if="importanceLabel"
          class="font-mono text-[10px] font-semibold tracking-wide px-1.5 py-0.5 rounded-sm border"
          :style="{
            color: importanceCssColor,
            borderColor: `color-mix(in oklch, ${importanceCssColor} 40%, transparent)`,
            background: `color-mix(in oklch, ${importanceCssColor} 10%, transparent)`
          }"
        >{{ importanceLabel }}</span>

        <!-- Score bar -->
        <template v-if="importanceScore != null">
          <div class="flex items-center gap-1 ml-auto">
            <div class="w-8 h-[3px] bg-gray-700 rounded-full overflow-hidden">
              <div
                class="h-full rounded-full transition-all duration-300"
                :style="{
                  width: `${importanceScore}%`,
                  background: importanceCssColor
                }"
              />
            </div>
            <span
              class="font-mono text-[10px] min-w-[20px] text-right"
              :style="{ color: importanceCssColor }"
            >{{ importanceScore }}</span>
          </div>
        </template>
      </div>
    </div>

    <!-- Right-side icons -->
    <div class="absolute right-3 top-3 flex gap-1.5 items-center z-10">
      <AnalysisIndicator :article-id="props.article.id"/>
      <ListOrderedIcon v-if="queueStore.isArticleQueued(props.article.id)" class="w-3.5 h-3.5 text-blue-400" title="Queued for analysis"/>
      <BookmarkIcon v-if="props.article.bookmarked" class="w-3.5 h-3.5 text-yellow-400"/>
    </div>
  </div>
</template>
