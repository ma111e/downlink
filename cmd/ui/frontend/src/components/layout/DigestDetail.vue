<script setup lang="ts">
import { computed, onMounted, provide, ref, watch, inject } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { GenerateDigest, GetDigest, GetDigestArticles } from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import { models } from "../../../wailsjs/go/models.ts";
import { GetArticle } from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useToast } from "@/components/ui/toast";
import { XIcon, LoaderIcon } from "lucide-vue-next";
import { useFormatters } from '@/composables/useFormatters';
import { readTag, readTagStyle, renderMarkdown, dupDotStyle } from '@/composables/useDigestFormatters';
import ArticleDetail from "@/components/layout/ArticleDetail.vue";
import Digest = models.Digest;
import DigestAnalysis = models.DigestAnalysis;
import ArticleAnalysis = models.ArticleAnalysis;
import Article = models.Article;

const route = useRoute();
const router = useRouter();
const { toast } = useToast();
const digest = ref<Digest | null>(null);
const articleTitles = ref<Record<string, string>>({});
const loading = ref(false);
const error = ref<string | null>(null);

// Article tabs state
const openArticleTabs = ref<{ id: string; title: string; article: Article | null }[]>([]);
const selectedArticleId = ref<string | number | undefined>(undefined);
const articleAnalysisTab = ref('brief');

const SYNTHESIS_TAB_ID = 'synthesis';

// Generate state
const isGenerating = ref(false);
const generateError = ref<string | null>(null);
const generateSuccess = ref<string | null>(null);
const timeWindow = ref(24);
const elapsedSeconds = ref(0);
let timerInterval: number | null = null;

const { convertNanoToHours } = useFormatters();

// Build articleId → DigestAnalysis lookup
const daByArticle = computed<Record<string, DigestAnalysis>>(() => {
  const map: Record<string, DigestAnalysis> = {};
  for (const da of (digest.value?.digest_analyses || [])) {
    map[da.article_id] = da;
  }
  return map;
});

// Share digest_analyses with sibling DigestList via provide/inject
const digestAnalyses = computed(() => digest.value?.digest_analyses || []);
provide('digestAnalyses', digestAnalyses);

const TAG_ORDER = ['Must Read', 'Should Read', 'May Read', 'Optional', 'Unscored'];

// Articles sorted by importance score desc, exposed for the synthesis panel TOC
const sortedArticles = computed(() => {
  if (!digest.value?.digest_analyses) return [];
  return [...digest.value.digest_analyses].sort((a, b) => {
    const sa = a.analysis?.importance_score ?? 0;
    const sb = b.analysis?.importance_score ?? 0;
    return sb - sa;
  });
});

// Articles grouped by read tag in display order
const articleGroups = computed(() => {
  const buckets: Record<string, DigestAnalysis[]> = {};
  for (const da of sortedArticles.value) {
    const score = da.analysis?.importance_score ?? 0;
    const tag = readTag(score);
    if (!buckets[tag]) buckets[tag] = [];
    buckets[tag].push(da);
  }
  return TAG_ORDER.filter(t => buckets[t]).map(t => ({ tag: t, items: buckets[t] }));
});

// Analysis for the currently selected article tab
const currentArticleAnalysis = computed<ArticleAnalysis | null>(() => {
  if (!selectedArticleId.value || selectedArticleId.value === SYNTHESIS_TAB_ID) return null;
  return daByArticle.value[selectedArticleId.value.toString()]?.analysis ?? null;
});

const fetchDigestData = async () => {
  const digestId = route.params.id as string;
  if (!digestId) return;

  try {
    loading.value = true;
    const [result, arts] = await Promise.all([GetDigest(digestId), GetDigestArticles(digestId)]);
    digest.value = result;
    const titles: Record<string, string> = {};
    for (const a of arts) titles[a.id] = a.title || 'Untitled';
    articleTitles.value = titles;
  } catch (err) {
    const msg = 'Failed to load digest data';
    error.value = msg;
    toast({ title: "Error", description: msg, variant: "destructive" });
    console.error(err);
  } finally {
    loading.value = false;
  }
};

const handleGenerateDigest = async () => {
  try {
    isGenerating.value = true;
    generateError.value = null;
    generateSuccess.value = null;
    elapsedSeconds.value = 0;
    if (timerInterval) clearInterval(timerInterval);
    timerInterval = window.setInterval(() => { elapsedSeconds.value++; }, 1000);

    const now = new Date();
    const start = new Date(now.getTime() - timeWindow.value * 3600 * 1000);
    const newDigest = await GenerateDigest(null, start, now, false, false, false, false, '', null);
    generateSuccess.value = 'Digest generated successfully!';
    toast({ title: "Success", description: "Digest generated successfully!" });
    await router.push(`/digest/${newDigest.id}`);
  } catch (err) {
    const msg = `Failed to generate digest: ${err}`;
    generateError.value = msg;
    toast({ title: "Error", description: msg, variant: "destructive" });
    console.error(err);
  } finally {
    isGenerating.value = false;
    if (timerInterval) { clearInterval(timerInterval); timerInterval = null; }
  }
};

const formatDate = (date: any) => new Date(date).toLocaleDateString('en-US', {
  year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
});

const formatElapsedTime = (seconds: number) => {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
};


// Article tabs
const addArticleTab = async (articleId: string) => {
  const existing = openArticleTabs.value.findIndex(t => t.id === articleId);
  if (existing !== -1) { selectedArticleId.value = articleId; return; }
  try {
    const article = await GetArticle(articleId);
    openArticleTabs.value.push({ id: articleId, title: article.title, article });
    selectedArticleId.value = articleId;
    articleAnalysisTab.value = 'brief';
  } catch {
    toast({ title: "Error", description: "Failed to load article", variant: "destructive" });
  }
};

const removeArticleTab = (articleId: string) => {
  const index = openArticleTabs.value.findIndex(t => t.id === articleId);
  if (index === -1) return;
  openArticleTabs.value.splice(index, 1);
  if (selectedArticleId.value === articleId) {
    selectedArticleId.value = openArticleTabs.value.length > 0
      ? openArticleTabs.value[Math.min(index, openArticleTabs.value.length - 1)].id
      : SYNTHESIS_TAB_ID;
  }
};

const selectArticleTab = (id: string | number) => { selectedArticleId.value = id; };

const parentSelectedArticleId = inject('selectedArticleId', ref<string | null>(null));
if (parentSelectedArticleId) {
  watch(parentSelectedArticleId, (newId) => {
    if (newId && route.path.includes('/digest/')) addArticleTab(newId);
  });
}

watch(() => route.params.id, (newId, oldId) => {
  if (newId !== oldId) {
    openArticleTabs.value = [];
    selectedArticleId.value = SYNTHESIS_TAB_ID;
    fetchDigestData();
  }
});

watch(() => route.path, (newPath) => {
  if (newPath === '/digests') { digest.value = null; articleTitles.value = {}; loading.value = false; error.value = null; }
});

onMounted(() => {
  fetchDigestData();
  selectedArticleId.value = SYNTHESIS_TAB_ID;
});

const timeWindowOptions = [
  { label: '12 hours', value: 12 },
  { label: '24 hours', value: 24 },
  { label: '48 hours', value: 48 },
  { label: '72 hours (3 days)', value: 72 },
  { label: '168 hours (7 days)', value: 168 },
];

// Expose to parent so DigestList can receive digestAnalyses via prop
defineExpose({ digestAnalyses });
</script>

<template>
  <div class="flex flex-col h-full flex-1 max-w-[60vw]">

    <!-- Generate Digest Card -->
    <Card class="mb-0 m-auto" v-if="!digest && !loading">
      <CardHeader>
        <CardTitle>Generate New Digest</CardTitle>
        <CardDescription>Create a new digest of your recent articles</CardDescription>
      </CardHeader>
      <CardContent>
        <div class="flex flex-col gap-4">
          <div>
            <label class="text-sm font-medium mb-2 block">Time Window (hours)</label>
            <Select v-model="timeWindow">
              <SelectTrigger class="w-full">
                <SelectValue placeholder="Select time window"/>
              </SelectTrigger>
              <SelectContent class="select-content-fix">
                <SelectItem v-for="option in timeWindowOptions" :key="option.value" :value="option.value">
                  <div class="select-item-fix">{{ option.label }}</div>
                </SelectItem>
              </SelectContent>
            </Select>
            <p class="text-sm text-muted-foreground mt-1">
              Articles published within this time window will be included
            </p>
          </div>
          <Alert variant="destructive" v-if="generateError">
            <AlertDescription>{{ generateError }}</AlertDescription>
          </Alert>
          <Alert variant="default" class="bg-green-900/20 border-green-700 text-green-400" v-if="generateSuccess">
            <AlertDescription>{{ generateSuccess }}</AlertDescription>
          </Alert>
        </div>
      </CardContent>
      <CardFooter class="flex items-center gap-3">
        <Button @click="handleGenerateDigest" :disabled="isGenerating">
          <span v-if="!isGenerating">Generate Digest</span>
          <div v-else class="flex items-center gap-2">
            <LoaderIcon class="h-4 w-4 animate-spin"/>
            <span>Generating...</span>
          </div>
        </Button>
        <div v-if="isGenerating" class="text-xs bg-primary-foreground/20 px-2 py-1 rounded-full">
          {{ formatElapsedTime(elapsedSeconds) }}
        </div>
      </CardFooter>
    </Card>

    <!-- Digest Detail View -->
    <div class="flex flex-col h-full" v-if="digest">

      <!-- Header: metadata only -->
      <Card class="border-0 border-b border-border rounded-none">
        <CardHeader class="pb-3">
          <CardTitle class="text-2xl">Digest Summary</CardTitle>
          <CardDescription class="flex flex-wrap gap-2 mt-2">
            <Badge variant="secondary">{{ formatDate(digest.created_at) }}</Badge>
            <Badge variant="secondary">{{ digest.article_count }} articles</Badge>
            <Badge variant="secondary">{{ convertNanoToHours(digest.time_window) }}h window</Badge>
          </CardDescription>
        </CardHeader>
      </Card>

      <!-- Article / Synthesis tab bar -->
      <div class="border-b border-border bg-muted/30 lg:min-w-[60vw]">
        <Tabs v-model="selectedArticleId" class="w-full" @update:modelValue="selectArticleTab">
          <TabsList class="flex overflow-x-auto px-2 py-1 bg-background border-0">
            <TabsTrigger
              :value="SYNTHESIS_TAB_ID"
              class="flex cursor-pointer items-center gap-1 min-w-fit px-4 py-2 font-medium"
              :class="selectedArticleId === SYNTHESIS_TAB_ID ? 'bg-primary text-primary-foreground shadow-sm' : 'hover:bg-accent'"
            >
              Synthesis
            </TabsTrigger>
            <TabsTrigger
              v-for="tab in openArticleTabs"
              :key="tab.id"
              :value="tab.id"
              class="flex items-center gap-1 min-w-0 px-3 py-2 mx-1 border-l border-r border-border/30"
              :class="selectedArticleId === tab.id ? 'bg-accent/80 shadow-sm' : 'hover:bg-accent/40'"
              @mousedown.middle.prevent="removeArticleTab(tab.id)"
            >
              <div class="flex items-center cursor-pointer min-w-0 w-full">
                <span class="truncate max-w-[150px]">{{ tab.title }}</span>
                <button
                  @click.stop="removeArticleTab(tab.id)"
                  class="ml-1 flex-shrink-0 cursor-pointer text-muted-foreground hover:text-white rounded-full h-full text-base font-bold"
                >
                  <XIcon class="h-4 w-4"/>
                </button>
              </div>
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <!-- Content area -->
      <div class="flex flex-1 overflow-auto w-full">

        <!-- Article tab selected -->
        <div
          v-if="selectedArticleId && selectedArticleId !== SYNTHESIS_TAB_ID"
          class="flex flex-col flex-1 h-full overflow-auto w-full"
        >
          <!-- Per-article analysis panel (only when analysis exists for this article) -->
          <div v-if="currentArticleAnalysis" class="border-b border-border bg-muted/20 flex-shrink-0">
            <!-- Metadata strip -->
            <div class="flex items-center gap-4 px-4 py-1.5 text-xs text-muted-foreground border-b border-border/50 flex-wrap">
              <span>
                <strong class="text-foreground/70">Provider</strong>
                {{ currentArticleAnalysis.provider_type }} / {{ currentArticleAnalysis.model_name }}
              </span>
              <span v-if="currentArticleAnalysis.importance_score > 0">
                <strong class="text-foreground/70">Score</strong>
                {{ currentArticleAnalysis.importance_score }}/100
              </span>
              <span
                v-if="currentArticleAnalysis.importance_score > 0"
                class="text-[10px] font-semibold px-1.5 py-0.5 rounded"
                :style="readTagStyle(readTag(currentArticleAnalysis.importance_score))"
              >
                {{ readTag(currentArticleAnalysis.importance_score) }}
              </span>
            </div>
            <!-- Analysis level tab buttons -->
            <div class="flex gap-0 px-2 pt-1">
              <button
                v-for="tab in ['brief', 'standard', 'full', 'keypoints', 'article']"
                :key="tab"
                class="px-3 py-1.5 text-xs font-medium border-b-2 transition-colors"
                :class="articleAnalysisTab === tab
                  ? 'border-blue-500 text-blue-400'
                  : 'border-transparent text-muted-foreground hover:text-foreground'"
                @click="articleAnalysisTab = tab"
              >
                {{ tab === 'keypoints' ? 'Key Points' : tab.charAt(0).toUpperCase() + tab.slice(1) }}
              </button>
            </div>
          </div>

          <!-- Analysis content -->
          <ScrollArea v-if="currentArticleAnalysis && articleAnalysisTab !== 'article'" class="flex-1 p-6 w-full">
            <div v-if="articleAnalysisTab === 'brief'"
              class="prose prose-invert max-w-none"
              v-html="renderMarkdown(currentArticleAnalysis.brief_overview)"
            />
            <div v-else-if="articleAnalysisTab === 'standard'"
              class="prose prose-invert max-w-none"
              v-html="renderMarkdown(currentArticleAnalysis.standard_synthesis)"
            />
            <div v-else-if="articleAnalysisTab === 'full'"
              class="prose prose-invert max-w-none"
              v-html="renderMarkdown(currentArticleAnalysis.comprehensive_synthesis)"
            />
            <div v-else-if="articleAnalysisTab === 'keypoints'">
              <ul v-if="currentArticleAnalysis.key_points?.length" class="space-y-2 mb-4">
                <li
                  v-for="(point, i) in currentArticleAnalysis.key_points"
                  :key="i"
                  class="flex gap-2 text-sm leading-relaxed"
                >
                  <span class="text-blue-400 font-bold flex-shrink-0">–</span>
                  <span>{{ point }}</span>
                </li>
              </ul>
              <template v-if="currentArticleAnalysis.insights?.length">
                <div class="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground mb-2 mt-4">
                  Insights
                </div>
                <ul class="space-y-2">
                  <li
                    v-for="(insight, i) in currentArticleAnalysis.insights"
                    :key="i"
                    class="flex gap-2 text-sm leading-relaxed"
                  >
                    <span class="text-blue-400 font-bold flex-shrink-0">–</span>
                    <span>{{ insight }}</span>
                  </li>
                </ul>
              </template>
            </div>
          </ScrollArea>

          <!-- Raw article content -->
          <div
            v-if="!currentArticleAnalysis || articleAnalysisTab === 'article'"
            class="flex-1 h-full overflow-auto w-full"
          >
            <ArticleDetail :articleId="selectedArticleId.toString()"/>
          </div>
        </div>

        <!-- Synthesis tab content -->
        <ScrollArea v-else class="flex-1 w-full">
          <div class="p-6 flex flex-col gap-6">

            <!-- Digest overview -->
            <div
              v-if="digest.digest_summary"
              class="border border-border border-l-2 border-l-blue-500 rounded bg-muted/30 px-4 py-3"
            >
              <div class="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground mb-2">Overview</div>
              <div class="prose prose-invert prose-sm max-w-none" v-html="renderMarkdown(digest.digest_summary)"/>
            </div>

            <!-- Articles grouped by read tag -->
            <div v-if="articleGroups.length" class="flex flex-col gap-5">
              <div v-for="group in articleGroups" :key="group.tag">
                <!-- Group label -->
                <div
                  class="text-[11px] font-semibold uppercase tracking-widest px-2 py-0.5 rounded inline-block mb-2"
                  :style="readTagStyle(group.tag)"
                >
                  {{ group.tag }}
                </div>
                <!-- Article rows -->
                <div class="flex flex-col gap-1">
                  <div
                    v-for="(da, i) in group.items"
                    :key="da.article_id"
                    class="flex items-start gap-3 px-3 py-2 rounded hover:bg-muted/40 cursor-pointer group"
                    @click="addArticleTab(da.article_id)"
                  >
                    <span class="text-[11px] font-mono text-muted-foreground flex-shrink-0 w-5 pt-0.5">{{ String(i + 1).padStart(2, '0') }}</span>
                    <span class="flex-1 text-sm leading-snug text-foreground group-hover:text-white">
                      {{ articleTitles[da.article_id] || 'Untitled' }}
                    </span>
                    <span
                      v-if="da.analysis?.importance_score"
                      class="text-[11px] font-mono text-muted-foreground flex-shrink-0 pt-0.5"
                    >{{ da.analysis.importance_score }}</span>
                    <span
                      v-if="da.duplicate_group"
                      class="inline-block w-2 h-2 rounded-full flex-shrink-0 mt-1"
                      :style="dupDotStyle(da.duplicate_group)"
                      :title="`Duplicate group: ${da.duplicate_group}`"
                    />
                  </div>
                </div>
              </div>
            </div>

          </div>
        </ScrollArea>

      </div>
    </div>

    <div v-else-if="loading" class="flex items-center justify-center h-full">
      <div class="text-xl text-muted-foreground">Loading digest...</div>
    </div>
  </div>
</template>
