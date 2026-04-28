<script setup lang="ts">
import { computed, inject, onMounted, ref, watch } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { ListDigests, GetDigestArticles } from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import { models } from "../../../wailsjs/go/models.ts";
import SearchBar from '@/components/SearchBar.vue';
import { ScrollArea } from "@/components/ui/scroll-area";
import { useToast } from "@/components/ui/toast";
import DigestListItem from '@/components/digests/DigestListItem.vue';
import { readTag, readTagStyle, dupDotStyle, sourceDotStyle } from '@/composables/useDigestFormatters';
import Digest = models.Digest;
import DigestAnalysis = models.DigestAnalysis;
import Article = models.Article;

const props = defineProps<{
  searchQuery?: string;
  selectedDigestId?: string;
}>();

// Injected from sibling DigestDetail (shared via router-view parent)
const injectedDigestAnalyses = inject('digestAnalyses', computed<DigestAnalysis[]>(() => []));

const emit = defineEmits<{
  (e: 'search', query: string): void;
  (e: 'article-selected', articleId: string): void;
}>();

const router = useRouter();
const route = useRoute();
const { toast } = useToast();
const digests = ref<Digest[]>([]);
const articles = ref<Article[]>([]);
const filteredArticles = ref<Article[]>([]);
const loading = ref(false);
const articlesLoading = ref(false);
const localSearchQuery = ref(props.searchQuery || '');

// Build a lookup map from articleId → DigestAnalysis for enrichment
const daByArticle = computed<Record<string, DigestAnalysis>>(() => {
  const map: Record<string, DigestAnalysis> = {};
  for (const da of (injectedDigestAnalyses.value || [])) {
    map[da.article_id] = da;
  }
  return map;
});

// Extract hostname from a URL for the source dot
function articleSource(link: string): string {
  try {
    return new URL(link).hostname.replace(/^www\./, '');
  } catch {
    return link;
  }
}

const fetchDigests = async () => {
  try {
    loading.value = true;
    digests.value = await ListDigests(30);

    if (props.searchQuery) {
      const query = props.searchQuery.toLowerCase();
      digests.value = digests.value.filter(digest =>
        digest.provider_results && digest.provider_results.some(result =>
          (result.brief_overview && result.brief_overview.toLowerCase().includes(query)) ||
          (result.standard_synthesis && result.standard_synthesis.toLowerCase().includes(query)) ||
          (result.comprehensive_synthesis && result.comprehensive_synthesis.toLowerCase().includes(query))
        )
      );
    }

    if (props.selectedDigestId) {
      await fetchDigestArticles(props.selectedDigestId);
    }
  } catch (err) {
    toast({ title: "Error", description: `Failed to load digests: ${err}`, variant: "destructive" });
    console.error(err);
  } finally {
    loading.value = false;
  }
};

const fetchDigestArticles = async (digestId: string) => {
  try {
    articlesLoading.value = true;
    articles.value = await GetDigestArticles(digestId);
    filterArticles();
  } catch (err) {
    toast({ title: "Error", description: `Failed to load digest articles: ${err}`, variant: "destructive" });
    console.error('Failed to load digest articles:', err);
  } finally {
    articlesLoading.value = false;
  }
};

const filterArticles = () => {
  if (!localSearchQuery.value) {
    filteredArticles.value = articles.value;
    return;
  }
  const query = localSearchQuery.value.toLowerCase();
  filteredArticles.value = articles.value.filter(article =>
    (article.title && article.title.toLowerCase().includes(query)) ||
    (article.content && article.content.toLowerCase().includes(query))
  );
};

const handleSearch = () => {
  if (props.selectedDigestId || route.params.id) {
    filterArticles();
  } else {
    emit('search', localSearchQuery.value);
  }
};

const selectArticle = (article: Article) => {
  if (route.path.includes('/digest/')) {
    emit('article-selected', article.id.toString());
  } else {
    router.push(`/article/${article.id}`);
  }
};

const selectDigest = (digest: Digest) => {
  router.push(`/digest/${digest.id}`);
};

watch(() => props.searchQuery, () => {
  localSearchQuery.value = props.searchQuery || '';
  if (props.selectedDigestId || route.params.id) {
    filterArticles();
  } else {
    fetchDigests();
  }
});

watch(localSearchQuery, () => {
  if (props.selectedDigestId || route.params.id) {
    filterArticles();
  }
});

watch(() => props.selectedDigestId, async (newDigestId) => {
  if (newDigestId) {
    await fetchDigestArticles(newDigestId);
  } else {
    articles.value = [];
  }
});

watch(() => route.params.id, async (newId) => {
  if (newId) {
    await fetchDigestArticles(newId as string);
  }
});

onMounted(async () => {
  await fetchDigests();
  if (props.selectedDigestId || route.params.id) {
    const digestId = props.selectedDigestId || route.params.id as string;
    await fetchDigestArticles(digestId);
  }
  filterArticles();
});
</script>

<template>
  <div class="min-w-76 w-[30vw] border-r border-gray-800 flex flex-col">
    <SearchBar
      v-model="localSearchQuery"
      :placeholder="(props.selectedDigestId || route.params.id) ? 'Search articles...' : 'Search digests...'"
      @search="handleSearch"
      :debounce="400"
    />

    <!-- Articles for selected digest -->
    <template v-if="props.selectedDigestId || route.params.id">
      <div class="px-3 py-2 border-b border-border text-xs font-semibold text-muted-foreground uppercase tracking-wide">
        Articles ({{ articles.length }})
      </div>
      <ScrollArea class="h-full">
        <div v-if="articlesLoading" class="p-4 text-center text-muted-foreground text-sm">
          Loading articles...
        </div>
        <div v-else-if="filteredArticles.length === 0" class="p-4 text-muted-foreground text-center text-sm">
          No articles found
        </div>
        <div
          v-else
          v-for="article in filteredArticles"
          :key="article.id"
          class="flex items-start gap-2 px-3 py-2.5 border-b border-border/60 hover:bg-muted/40 cursor-pointer"
          @click="selectArticle(article)"
        >
          <!-- Source dot -->
          <span
            class="mt-1 flex-shrink-0 inline-block w-2 h-2 rounded-full"
            :style="sourceDotStyle(articleSource(article.link || ''))"
            :title="articleSource(article.link || '')"
          />

          <div class="flex-1 min-w-0">
            <div class="flex items-start gap-1.5">
              <span class="text-sm font-medium leading-snug line-clamp-2 text-foreground flex-1 min-w-0">
                {{ article.title || 'Untitled' }}
              </span>
              <div class="flex items-center gap-1 flex-shrink-0 pt-0.5">
                <!-- Read tag badge -->
                <span
                  v-if="daByArticle[article.id]?.analysis?.importance_score"
                  class="text-[10px] font-semibold px-1.5 py-0.5 rounded whitespace-nowrap"
                  :style="readTagStyle(readTag(daByArticle[article.id].analysis!.importance_score))"
                >
                  {{ readTag(daByArticle[article.id].analysis!.importance_score) }}
                </span>
                <!-- Score -->
                <span
                  v-if="daByArticle[article.id]?.analysis?.importance_score"
                  class="text-[10px] text-muted-foreground font-mono"
                >
                  {{ daByArticle[article.id].analysis!.importance_score }}
                </span>
              </div>
            </div>
            <!-- Duplicate group dot -->
            <div v-if="daByArticle[article.id]?.duplicate_group" class="mt-1">
              <span
                class="inline-block w-2 h-2 rounded-full"
                :class="daByArticle[article.id].is_most_comprehensive ? 'ring-1 ring-offset-1 ring-current' : ''"
                :style="dupDotStyle(daByArticle[article.id].duplicate_group!)"
                :title="`Duplicate group: ${daByArticle[article.id].duplicate_group}`"
              />
            </div>
          </div>
        </div>
      </ScrollArea>
    </template>

    <!-- Digest list -->
    <template v-else>
      <ScrollArea class="h-full">
        <div v-if="loading" class="p-4 pt-8 text-center text-muted-foreground text-sm">
          Loading digests...
        </div>
        <div v-else-if="!digests || digests.length === 0" class="p-4 pt-8 text-center text-muted-foreground text-sm">
          No digests found
        </div>
        <template v-else>
          <DigestListItem
            v-for="digest in digests"
            :key="digest.id"
            :digest="digest"
            :is-selected="route.params.id === digest.id"
            @select="selectDigest"
          />
        </template>
      </ScrollArea>
    </template>
  </div>
</template>
