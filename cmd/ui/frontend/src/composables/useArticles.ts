import { reactive } from 'vue';
import { models } from "../../wailsjs/go/models.ts";
import {
  GetArticle,
  GetArticleCounts,
  ListArticles,
  MarkFeedArticlesRead,
  UpdateArticle,
} from "../../wailsjs/go/downlinkclient/DownlinkClient";

import Article = models.Article;
import ArticleCounts = models.ArticleCounts;
import ArticleFilter = models.ArticleFilter;
import Category = models.Category;
import RelatedArticle = models.RelatedArticle;
import Tag = models.Tag;

const DEFAULT_PAGE_SIZE = 30;
const MAX_PAGE_SIZE = 50;

type DateInput = Date | string | undefined;

type ArticleSource = {
  id: string;
  feed_id: string;
  title: string;
  content: string;
  link: string;
  published_at: unknown;
  fetched_at: unknown;
  read?: boolean;
  tags: Tag[];
  category_name?: string;
  category?: Category;
  hero_image?: string;
  bookmarked?: boolean;
  related_articles?: RelatedArticle[];
  latest_importance_score?: number;
};

export type ArticlePatch = Partial<Pick<ArticleSource,
  'read' |
  'bookmarked' |
  'category_name' |
  'category' |
  'hero_image' |
  'related_articles' |
  'latest_importance_score' |
  'tags' |
  'title' |
  'content' |
  'link'
>>;

export type ArticleQueryInput = {
  unread_only?: boolean;
  category_name?: string;
  tag_id?: string;
  bookmarked_only?: boolean;
  related_to_id?: string;
  start_date?: DateInput;
  end_date?: DateInput;
  feed_id?: string;
  exclude_digested?: boolean;
  offset?: number;
  limit?: number;
  query?: string;
};

type CachedArticleList = {
  signature: string;
  filter: ArticleFilter;
  items: Article[];
  offset: number;
  limit: number;
  hasMore: boolean;
  loadingInitial: boolean;
  loadingMore: boolean;
  loadedCount: number;
};

function clampLimit(limit?: number): number {
  if (!limit || limit <= 0) return DEFAULT_PAGE_SIZE;
  return Math.min(limit, MAX_PAGE_SIZE);
}

function serializeDate(value: unknown): string | undefined {
  if (!value) return undefined;
  if (value instanceof Date) return value.toISOString();
  if (typeof value === 'string') return value;
  if (typeof value === 'object' && value !== null && 'toISOString' in value && typeof value.toISOString === 'function') {
    return value.toISOString();
  }
  return undefined;
}

function articleToSource(article: Article): ArticleSource {
  return {
    id: article.id,
    feed_id: article.feed_id,
    title: article.title,
    content: article.content,
    link: article.link,
    published_at: article.published_at,
    fetched_at: article.fetched_at,
    read: article.read,
    tags: article.tags,
    category_name: article.category_name,
    category: article.category,
    hero_image: article.hero_image,
    bookmarked: article.bookmarked,
    related_articles: article.related_articles,
    latest_importance_score: article.latest_importance_score,
  };
}

function hydrateArticle(article: Article | ArticleSource): Article {
  return article instanceof models.Article ? article : new models.Article(article);
}

function hydrateArticles(articles: Array<Article | ArticleSource>): Article[] {
  return articles.map(hydrateArticle);
}

function applyArticlePatch(article: Article, patch: ArticlePatch): Article {
  return new models.Article({
    ...articleToSource(article),
    ...patch,
  });
}

function articleFilterToQueryInput(filter: ArticleFilter): ArticleQueryInput {
  return {
    unread_only: Boolean(filter.unread_only),
    category_name: filter.category_name || '',
    tag_id: filter.tag_id || '',
    bookmarked_only: Boolean(filter.bookmarked_only),
    related_to_id: filter.related_to_id || '',
    start_date: serializeDate(filter.start_date),
    end_date: serializeDate(filter.end_date),
    feed_id: filter.feed_id || '',
    exclude_digested: Boolean(filter.exclude_digested),
    offset: Number(filter.offset || 0),
    limit: clampLimit(Number(filter.limit || DEFAULT_PAGE_SIZE)),
    query: (filter.query || '').trim(),
  };
}

function normalizeFilter(filter: ArticleQueryInput = {}): ArticleFilter {
  return new models.ArticleFilter({
    unread_only: Boolean(filter.unread_only),
    category_name: filter.category_name || '',
    tag_id: filter.tag_id || '',
    bookmarked_only: Boolean(filter.bookmarked_only),
    related_to_id: filter.related_to_id || '',
    start_date: serializeDate(filter.start_date),
    end_date: serializeDate(filter.end_date),
    feed_id: filter.feed_id || '',
    exclude_digested: Boolean(filter.exclude_digested),
    offset: Math.max(0, Number(filter.offset || 0)),
    limit: clampLimit(Number(filter.limit || DEFAULT_PAGE_SIZE)),
    query: (filter.query || '').trim(),
  });
}

function normalizeCountsFilter(filter: ArticleQueryInput = {}): ArticleFilter {
  return normalizeFilter({
    query: filter.query || '',
    start_date: filter.start_date,
    end_date: filter.end_date,
    exclude_digested: filter.exclude_digested,
    limit: DEFAULT_PAGE_SIZE,
    offset: 0,
  });
}

function filterSignature(filter: ArticleFilter): string {
  return JSON.stringify({
    unread_only: Boolean(filter.unread_only),
    category_name: filter.category_name || '',
    tag_id: filter.tag_id || '',
    bookmarked_only: Boolean(filter.bookmarked_only),
    related_to_id: filter.related_to_id || '',
    start_date: serializeDate(filter.start_date),
    end_date: serializeDate(filter.end_date),
    feed_id: filter.feed_id || '',
    exclude_digested: Boolean(filter.exclude_digested),
    limit: clampLimit(Number(filter.limit || DEFAULT_PAGE_SIZE)),
    query: (filter.query || '').trim(),
  });
}

function createCacheEntry(signature: string, filter: ArticleFilter): CachedArticleList {
  return {
    signature,
    filter,
    items: [],
    offset: 0,
    limit: clampLimit(Number(filter.limit || DEFAULT_PAGE_SIZE)),
    hasMore: true,
    loadingInitial: false,
    loadingMore: false,
    loadedCount: 0,
  };
}

const articleById = reactive<Record<string, Article>>({});

const state = reactive({
  caches: {} as Record<string, CachedArticleList>,
  currentKey: '',
  currentFilter: null as ArticleFilter | null,
  currentArticleLoading: false,
  error: '',
  counts: null as ArticleCounts | null,
  countsLoading: false,
  countsFilter: null as ArticleQueryInput | null,
  get currentList(): CachedArticleList | null {
    return this.currentKey ? (this.caches[this.currentKey] ?? null) : null;
  },
  get articles(): Article[] {
    return this.currentList?.items ?? [];
  },
  get loading(): boolean {
    return this.currentArticleLoading || Boolean(this.currentList?.loadingInitial);
  },
  get loadingMore(): boolean {
    return Boolean(this.currentList?.loadingMore);
  },
  get hasMore(): boolean {
    return Boolean(this.currentList?.hasMore);
  },
});

function reindexArticles(articles: Article[]) {
  for (const article of articles) {
    articleById[article.id] = article;
  }
}

function replaceArticleAcrossCaches(id: string, patch: ArticlePatch): Article | null {
  let updatedArticle: Article | null = null;

  if (articleById[id]) {
    updatedArticle = applyArticlePatch(articleById[id], patch);
    articleById[id] = updatedArticle;
  }

  for (const key of Object.keys(state.caches)) {
    const cache = state.caches[key];
    const index = cache.items.findIndex(article => article.id === id);
    if (index !== -1) {
      const nextArticle = applyArticlePatch(cache.items[index], patch);
      cache.items[index] = nextArticle;
      articleById[id] = nextArticle;
      updatedArticle = nextArticle;
    }
  }

  return updatedArticle;
}

function mergeArticleIntoCaches(article: Article) {
  const hydratedArticle = hydrateArticle(article);
  articleById[hydratedArticle.id] = hydratedArticle;

  for (const key of Object.keys(state.caches)) {
    const cache = state.caches[key];
    const index = cache.items.findIndex(entry => entry.id === hydratedArticle.id);
    if (index !== -1) {
      cache.items[index] = hydrateArticle(articleToSource(hydratedArticle));
    }
  }
}

async function refreshCountsIfActive() {
  if (!state.countsFilter) return;
  try {
    await fetchArticleCounts(state.countsFilter);
  } catch (error) {
    console.error('Failed to refresh article counts:', error);
  }
}

async function fetchArticleCounts(filter: ArticleQueryInput = {}) {
  const normalized = normalizeCountsFilter(filter);
  state.countsFilter = articleFilterToQueryInput(normalized);
  state.countsLoading = true;

  try {
    state.counts = new models.ArticleCounts(await GetArticleCounts(normalized));
    return state.counts;
  } catch (err) {
    console.error(err);
    throw err;
  } finally {
    state.countsLoading = false;
  }
}

export function useArticles() {
  const loadInitialArticles = async (filter: ArticleQueryInput = {}) => {
    const normalized = normalizeFilter(filter);
    const signature = filterSignature(normalized);

    state.currentKey = signature;
    state.currentFilter = normalized;

    let cache = state.caches[signature];
    if (!cache) {
      cache = createCacheEntry(signature, normalized);
      state.caches[signature] = cache;
    } else {
      cache.filter = normalized;
      cache.limit = clampLimit(Number(normalized.limit || cache.limit));
    }

    if (cache.items.length > 0) {
      return cache.items;
    }

    try {
      cache.loadingInitial = true;
      state.error = '';

      const requestFilter = normalizeFilter({
        ...articleFilterToQueryInput(normalized),
        offset: 0,
        limit: cache.limit,
      });
      const articles = hydrateArticles(await ListArticles(requestFilter));

      cache.items = articles;
      cache.offset = articles.length;
      cache.loadedCount = articles.length;
      cache.hasMore = articles.length === cache.limit;
      reindexArticles(articles);

      return cache.items;
    } catch (err) {
      state.error = 'Failed to load articles';
      console.error(err);
      return [];
    } finally {
      cache.loadingInitial = false;
    }
  };

  const loadMoreArticles = async () => {
    const cache = state.currentList;
    if (!cache || cache.loadingMore || cache.loadingInitial || !cache.hasMore) {
      return cache?.items ?? [];
    }

    try {
      cache.loadingMore = true;
      state.error = '';

      const requestFilter = normalizeFilter({
        ...articleFilterToQueryInput(cache.filter),
        offset: cache.offset,
        limit: cache.limit,
      });
      const nextPage = hydrateArticles(await ListArticles(requestFilter));

      const seenIds = new Set(cache.items.map(article => article.id));
      const appended = nextPage.filter(article => !seenIds.has(article.id));
      cache.items = [...cache.items, ...appended];
      cache.offset += nextPage.length;
      cache.loadedCount = cache.items.length;
      cache.hasMore = nextPage.length === cache.limit;
      reindexArticles(appended);

      return cache.items;
    } catch (err) {
      state.error = 'Failed to load more articles';
      console.error(err);
      return cache.items;
    } finally {
      cache.loadingMore = false;
    }
  };

  const refreshCurrentList = async () => {
    const cache = state.currentList;
    if (!cache) return [];

    const targetCount = Math.max(cache.loadedCount || 0, cache.limit);
    const pageSize = cache.limit;

    try {
      cache.loadingInitial = true;
      state.error = '';

      const refreshed: Article[] = [];
      let offset = 0;
      let lastPageLength = 0;

      while (refreshed.length < targetCount) {
        const requestFilter = normalizeFilter({
          ...articleFilterToQueryInput(cache.filter),
          offset,
          limit: pageSize,
        });
        const page = hydrateArticles(await ListArticles(requestFilter));
        lastPageLength = page.length;

        if (page.length === 0) {
          break;
        }

        refreshed.push(...page);
        offset += page.length;

        if (page.length < pageSize) {
          break;
        }
      }

      const nextItems = refreshed.slice(0, targetCount);

      cache.items = nextItems;
      cache.offset = nextItems.length;
      cache.loadedCount = nextItems.length;
      cache.hasMore = refreshed.length > targetCount || lastPageLength === pageSize;
      reindexArticles(nextItems);

      return cache.items;
    } catch (err) {
      state.error = 'Failed to refresh articles';
      console.error(err);
      return cache.items;
    } finally {
      cache.loadingInitial = false;
    }
  };

  const invalidateCurrentList = () => {
    if (state.currentKey) {
      delete state.caches[state.currentKey];
    }
    state.currentKey = '';
    state.currentFilter = null;
  };

  const getCachedArticle = (id: string) => {
    return articleById[id] ?? null;
  };

  const getArticle = async (id: string) => {
    try {
      state.currentArticleLoading = true;
      const article = hydrateArticle(await GetArticle(id));
      mergeArticleIntoCaches(article);

      if (!article.read) {
        const marked = await markArticleAsRead(id);
        if (marked) {
          article.read = true;
        }
      }

      return article;
    } catch (err) {
      state.error = 'Failed to load article';
      console.error(err);
      return null;
    } finally {
      state.currentArticleLoading = false;
    }
  };

  const markArticleAsRead = async (id: string) => {
    try {
      await UpdateArticle(id, new models.ArticleUpdate({ read: true }));
      replaceArticleAcrossCaches(id, { read: true });
      void refreshCountsIfActive();
      return true;
    } catch (err) {
      console.error('Failed to mark article as read:', err);
      return false;
    }
  };

  const markArticleAsUnread = async (id: string) => {
    try {
      await UpdateArticle(id, new models.ArticleUpdate({ read: false }));
      replaceArticleAcrossCaches(id, { read: false });
      void refreshCountsIfActive();
      return true;
    } catch (err) {
      console.error('Failed to mark article as unread:', err);
      return false;
    }
  };

  const toggleArticleBookmark = async (id: string, currentStatus: boolean) => {
    try {
      await UpdateArticle(id, new models.ArticleUpdate({ bookmarked: !currentStatus }));
      replaceArticleAcrossCaches(id, { bookmarked: !currentStatus });
      void refreshCountsIfActive();
      return true;
    } catch (err) {
      console.error('Failed to toggle bookmark status:', err);
      return false;
    }
  };

  const markAllArticlesAsRead = async (feedId: string) => {
    try {
      await MarkFeedArticlesRead(feedId);

      for (const id of Object.keys(articleById)) {
        const article = articleById[id];
        if (article.feed_id === feedId) {
          articleById[id] = applyArticlePatch(article, { read: true });
        }
      }

      for (const key of Object.keys(state.caches)) {
        const cache = state.caches[key];
        cache.items = cache.items.map(article => (
          article.feed_id === feedId ? applyArticlePatch(article, { read: true }) : article
        ));
        cache.loadedCount = cache.items.length;
        reindexArticles(cache.items);
      }
      void refreshCountsIfActive();
      return true;
    } catch (err) {
      console.error('Error marking all articles as read:', err);
      throw err;
    }
  };

  const toggleArticleReadStatus = async (id: string, currentStatus: boolean) => {
    return currentStatus ? markArticleAsUnread(id) : markArticleAsRead(id);
  };

  const updateArticle = async (id: string, updates: ArticlePatch) => {
    const articleUpdate = new models.ArticleUpdate({
      read: updates.read,
      tag_ids: updates.tags?.map(tag => tag.id),
      category_name: updates.category_name,
      hero_image: updates.hero_image,
      bookmarked: updates.bookmarked,
      related_articles: updates.related_articles,
    });

    try {
      await UpdateArticle(id, articleUpdate);
      const updatedArticle = replaceArticleAcrossCaches(id, updates);
      void refreshCountsIfActive();
      return updatedArticle;
    } catch (err) {
      console.error('Failed to update article:', err);
      return null;
    }
  };

  const fetchArticles = async (filter?: ArticleQueryInput) => {
    if (filter) {
      return loadInitialArticles(filter);
    }
    return refreshCurrentList();
  };

  return {
    state,
    loadInitialArticles,
    loadMoreArticles,
    refreshCurrentList,
    invalidateCurrentList,
    fetchArticleCounts,
    fetchArticles,
    getCachedArticle,
    getArticle,
    markArticleAsRead,
    markAllArticlesAsRead,
    markArticleAsUnread,
    toggleArticleReadStatus,
    toggleArticleBookmark,
    updateArticle,
  };
}
