<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { Plus, Edit2, Trash2, Save, X } from 'lucide-vue-next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { useFeeds } from '@/composables/useFeeds';
import { useConfig } from '@/composables/useConfig';
import {DeleteFeed, RegisterFeed, SaveConfig} from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import {models} from "../../../wailsjs/go/models.ts";
import FeedConfig = models.FeedConfig;
import {useToast} from "@/components/ui/toast";

const { feeds, fetchFeeds, loading: feedsLoading } = useFeeds();
const { config, loading: configLoading, fetchConfig } = useConfig();

// Form data for adding new feed
const feedUrl = ref('');
const feedTitle = ref('');
const feedType = ref('rss');
const feedEnabled = ref(true);
const feedDynamicScraping = ref(false);
const feedFullBrowser = ref(false);
// Selectors for new feed
const feedArticleSelector = ref('');
const feedCutoffSelector = ref('');
const feedBlacklistSelector = ref('');
// Full browser triggers for new feed (comma-separated CSS selectors)
const feedTriggersLoaded = ref('');
const feedTriggersFailed = ref('');
const isSubmitting = ref(false);
const errorMessage = ref('');

// Edit mode state
const editingFeedId = ref<string | null>(null);
const editFeedUrl = ref('');
const editFeedTitle = ref('');
const editFeedEnabled = ref(true);
const editFeedDynamicScraping = ref(false);
const editFeedFullBrowser = ref(false);
// Selectors for edit mode
const editArticleSelector = ref('');
const editCutoffSelector = ref('');
const editBlacklistSelector = ref('');
// Full browser triggers for edit mode (comma-separated CSS selectors)
const editTriggersLoaded = ref('');
const editTriggersFailed = ref('');

// Parse comma-separated selector string to array, filtering empty entries
const parseSelectors = (s: string): string[] =>
  s.split(',').map(v => v.trim()).filter(v => v.length > 0);

// Mutual exclusivity helpers
const setAddScraping = (mode: 'dynamic' | 'full_browser' | null) => {
  feedDynamicScraping.value = mode === 'dynamic';
  feedFullBrowser.value = mode === 'full_browser';
};
const setEditScraping = (mode: 'dynamic' | 'full_browser' | null) => {
  editFeedDynamicScraping.value = mode === 'dynamic';
  editFeedFullBrowser.value = mode === 'full_browser';
};

const {toast} = useToast();

// Load feeds and models when component mounts
onMounted(async () => {
  await fetchFeeds();
  await fetchConfig();
});

// Find feed by Id
const findFeedById = (id: string) => {
  return feeds.value.find(feed => feed.id === id);
};

// Start editing a feed
const startEditing = (feedId: string) => {
  const feed = findFeedById(feedId);
  if (feed) {
    editingFeedId.value = feedId;
    editFeedUrl.value = feed.url;
    editFeedTitle.value = feed.title || '';
    editFeedEnabled.value = feed.enabled || true;

    const scrapingMode = feed.scraper?.['scraping'] as string || '';
    setEditScraping(scrapingMode === 'dynamic' ? 'dynamic' : scrapingMode === 'full_browser' ? 'full_browser' : null);

    // Load selectors from scraper map if available
    editArticleSelector.value = feed.scraper?.['article'] as string || '';
    editCutoffSelector.value = feed.scraper?.['cutoff'] as string || '';
    editBlacklistSelector.value = feed.scraper?.['blacklist'] as string || '';

    // Load full browser triggers
    const triggers = feed.scraper?.['triggers'] as { loaded?: string[], failed?: string[] } | undefined;
    editTriggersLoaded.value = (triggers?.loaded ?? []).join(', ');
    editTriggersFailed.value = (triggers?.failed ?? []).join(', ');
  }
};

// Cancel editing
const cancelEditing = () => {
  editingFeedId.value = null;
};

// Save edited feed
const saveEditedFeed = async (feedId: string) => {
  try {
    isSubmitting.value = true;
    errorMessage.value = '';

    if (!config.value) {
      errorMessage.value = 'Configuration not loaded';
      return;
    }

    // Find the feed to update in the models
    const feed = findFeedById(feedId);
    if (!feed) {
      errorMessage.value = 'Feed not found';
      return;
    }

    const feedIndex = config.value.feeds.findIndex((f: FeedConfig) => f.url === feed.url && f.title === feed.title);

    if (feedIndex !== -1) {
      // Update the feed properties
      config.value.feeds[feedIndex].url = editFeedUrl.value;
      config.value.feeds[feedIndex].title = editFeedTitle.value;
      config.value.feeds[feedIndex].enabled = editFeedEnabled.value;
      config.value.feeds[feedIndex].scraping = editFeedFullBrowser.value ? 'full_browser' : editFeedDynamicScraping.value ? 'dynamic' : '';
      // Build scraper map from CSS selectors and full browser triggers
      const scraper: Record<string, any> = {};
      if (editArticleSelector.value) {
        scraper['article'] = editArticleSelector.value;
      }
      if (editCutoffSelector.value) {
        scraper['cutoff'] = editCutoffSelector.value;
      }
      if (editBlacklistSelector.value) {
        scraper['blacklist'] = editBlacklistSelector.value;
      }
      if (editFeedFullBrowser.value) {
        const loaded = parseSelectors(editTriggersLoaded.value);
        const failed = parseSelectors(editTriggersFailed.value);
        if (loaded.length > 0 || failed.length > 0) {
          scraper['triggers'] = { loaded, failed };
        }
      }
      config.value.feeds[feedIndex].scraper = Object.keys(scraper).length > 0 ? scraper : undefined;

      // Save the updated models
      await SaveConfig(config.value);

      // Refresh feeds and models to reflect changes
      await Promise.all([fetchFeeds(), fetchConfig()]);

      // Exit edit mode
      editingFeedId.value = null;
    } else {
      errorMessage.value = 'Unable to find feed in configuration';
    }
  } catch (error) {
    console.error('Failed to update feed:', error);
    errorMessage.value = `Failed to update feed: ${error}`;
  } finally {
    isSubmitting.value = false;
  }
};

// Toggle feed enabled status
const toggleFeedEnabled = async (feedId: string) => {
  try {
    if (!config.value) {
      errorMessage.value = 'Configuration not loaded';
      return;
    }

    // Find the feed to update
    const feed = findFeedById(feedId);
    if (!feed) {
      errorMessage.value = 'Feed not found';
      return;
    }

    const feedIndex = config.value.feeds.findIndex(f =>
      f.url === feed.url && f.title === feed.title);

    if (feedIndex !== -1) {
      // Toggle the enabled status
      config.value.feeds[feedIndex].enabled = !feed.enabled;

      // Save the updated models
      await SaveConfig(config.value);

      // Refresh feeds and models to reflect changes
      await Promise.all([fetchFeeds(), fetchConfig()]);
    } else {
      errorMessage.value = 'Unable to find feed in configuration';
    }
  } catch (error) {
    console.error('Failed to toggle feed status:', error);
    errorMessage.value = `Failed to update feed: ${error}`;
  }
};

// Delete a feed
const deleteFeed = async (feedId: string) => {
  if (!confirm('Are you sure you want to delete this feed? This will remove all associated articles.')) {
    return;
  }

  try {
    isSubmitting.value = true;
    errorMessage.value = '';

    if (!config.value) {
      errorMessage.value = 'Configuration not loaded';
      return;
    }

    // Find the feed to delete
    const feed = findFeedById(feedId);
    if (!feed) {
      errorMessage.value = 'Feed not found';
      return;
    }

    // Call the backend to remove the feed and all its articles
    await DeleteFeed(feedId);

    // Remove the feed from models
    config.value.feeds = config.value.feeds.filter(f =>
      !(f.url === feed.url && f.title === feed.title));

    // Save the updated models
    await SaveConfig(config.value);

    // Refresh feeds and models to reflect changes
    await Promise.all([fetchFeeds(), fetchConfig()]);
  } catch (error) {
    console.error('Failed to delete feed:', error);
    errorMessage.value = `Failed to delete feed: ${error}`;
  } finally {
    isSubmitting.value = false;
  }
};

// Submit new feed form
  /**
   * Handles adding a new feed.
   *
   * Submits the new feed form. Adds the new feed to the models and saves the updated models.
   * Registers the new feed with the backend.
   * Refreshes the feeds and models lists to reflect the new feed.
   * Resets the form fields.
   */
const addNewFeed = async () => {
  if (!feedUrl.value) {
    errorMessage.value = 'Feed URL is required';
    return;
  }

  try {
    isSubmitting.value = true;
    errorMessage.value = '';

    if (!config.value) {
      errorMessage.value = 'Configuration not loaded';
      return;
    }

    // Create feed models object
    const scraper: Record<string, any> = {};
    if (feedArticleSelector.value) {
      scraper['article'] = feedArticleSelector.value;
    }
    if (feedCutoffSelector.value) {
      scraper['cutoff'] = feedCutoffSelector.value;
    }
    if (feedBlacklistSelector.value) {
      scraper['blacklist'] = feedBlacklistSelector.value;
    }
    if (feedFullBrowser.value) {
      const loaded = parseSelectors(feedTriggersLoaded.value);
      const failed = parseSelectors(feedTriggersFailed.value);
      if (loaded.length > 0 || failed.length > 0) {
        scraper['triggers'] = { loaded, failed };
      }
    }

    const newFeed = <FeedConfig>{
      url: feedUrl.value,
      type: feedType.value,
      title: feedTitle.value,
      enabled: feedEnabled.value,
      scraping: feedFullBrowser.value ? 'full_browser' : feedDynamicScraping.value ? 'dynamic' : '',
      scraper: Object.keys(scraper).length > 0 ? scraper : undefined,
    };

    // Add new feed to models
    config.value.feeds.push(newFeed);

    // Save updated models
    await SaveConfig(config.value);

    // Register the new feed
    await RegisterFeed(newFeed);

    toast({
      title: "Feed added",
      description: `Refreshing feed...`,
      variant: "default"
    });

    // Refresh feeds and models to reflect changes
    await Promise.all([fetchFeeds(), fetchConfig()]);

    // Reset form
    resetForm();
  } catch (error) {
    console.error('Failed to add feed:', error);
    errorMessage.value = `Failed to add feed: ${error}`;
  } finally {
    isSubmitting.value = false;
  }
};

// Reset form fields
const resetForm = () => {
  feedUrl.value = '';
  feedTitle.value = '';
  feedType.value = 'rss';
  feedEnabled.value = true;
  feedDynamicScraping.value = false;
  feedFullBrowser.value = false;
  feedArticleSelector.value = '';
  feedCutoffSelector.value = '';
  feedBlacklistSelector.value = '';
  feedTriggersLoaded.value = '';
  feedTriggersFailed.value = '';
  errorMessage.value = '';
};
</script>

<template>
  <div>
    <section class="mb-8">
      <h2 class="text-lg font-medium mb-4">Add New Feed</h2>
      <div class="space-y-4 bg-gray-900 p-4 rounded-lg">
        <div class="grid gap-2">
          <Label for="url">Feed URL</Label>
          <Input id="url" v-model="feedUrl" required placeholder="https://example.com/feed.xml" />
        </div>

        <div class="grid gap-2">
          <Label for="title">Feed Title (Optional)</Label>
          <Input id="title" v-model="feedTitle" placeholder="My Feed" />
        </div>

        <div class="flex items-center space-x-2">
          <Switch id="enabled" v-model="feedEnabled" />
          <Label for="enabled">Enable feed</Label>
        </div>
        
        <div class="flex items-center space-x-2">
          <Switch
            id="dynamic-scraping"
            :model-value="feedDynamicScraping"
            @update:modelValue="setAddScraping(feedDynamicScraping ? null : 'dynamic')"
          />
          <Label for="dynamic-scraping">Playwright (dynamic scraping)</Label>
          <div class="ml-2 text-xs text-gray-400">For JavaScript-heavy sites</div>
        </div>

        <div class="flex items-center space-x-2">
          <Switch
            id="full-browser"
            :model-value="feedFullBrowser"
            @update:modelValue="setAddScraping(feedFullBrowser ? null : 'full_browser')"
          />
          <Label for="full-browser">Full Browser (Solimen)</Label>
          <div class="ml-2 text-xs text-gray-400">Full browser rendering via Solimen</div>
        </div>

        <template v-if="feedFullBrowser">
          <h3 class="text-sm font-medium mt-2">Browser Triggers</h3>
          <div class="grid gap-2">
            <Label for="triggers-loaded">Loaded Selectors</Label>
            <Input id="triggers-loaded" v-model="feedTriggersLoaded" placeholder=".article-body, #content" />
            <p class="text-xs text-gray-400">Comma-separated CSS selectors — all must match for the page to be considered loaded</p>
          </div>
          <div class="grid gap-2">
            <Label for="triggers-failed">Failed Selectors</Label>
            <Input id="triggers-failed" v-model="feedTriggersFailed" placeholder=".error-page, #captcha" />
            <p class="text-xs text-gray-400">Comma-separated CSS selectors — all must match to indicate a load failure</p>
          </div>
        </template>

        <h3 class="text-sm font-medium mt-4">Content Extraction Selectors</h3>
        <div class="grid gap-2">
          <Label for="article-selector">Article Selector</Label>
          <Input id="article-selector" v-model="feedArticleSelector" placeholder="article, .article-content, #main-content" />
          <p class="text-xs text-gray-400">CSS selector to find the article content</p>
        </div>
        
        <div class="grid gap-2">
          <Label for="cutoff-selector">Cutoff Selector</Label>
          <Input id="cutoff-selector" v-model="feedCutoffSelector" placeholder=".comments, .social-icons" />
          <p class="text-xs text-gray-400">CSS selector to mark where to cutoff the article</p>
        </div>
        
        <div class="grid gap-2">
          <Label for="blacklist-selector">Blacklist Selector</Label>
          <Input id="blacklist-selector" v-model="feedBlacklistSelector" placeholder="script, style, nav, .ads" />
          <p class="text-xs text-gray-400">Elements to exclude from the article</p>
        </div>

        <div v-if="errorMessage" class="text-red-500 text-sm">
          {{ errorMessage }}
        </div>

        <div class="flex justify-end gap-2">
          <Button variant="outline" @click="resetForm">Reset</Button>
          <Button type="submit" @click="addNewFeed" :disabled="isSubmitting">
            <Plus class="h-4 w-4 mr-2" v-if="!isSubmitting" />
            <span v-if="isSubmitting">Adding...</span>
            <span v-else>Add Feed</span>
          </Button>
        </div>
      </div>
    </section>

    <section class="mb-8">
      <h2 class="text-lg font-medium mb-4">Current Feeds</h2>
      <div class="bg-gray-900 rounded-lg divide-y divide-gray-800">
        <div v-if="feedsLoading || configLoading" class="p-4 text-gray-500 text-center">
          Loading feeds...
        </div>
        <div v-else-if="feeds.length === 0" class="p-4 text-gray-500 text-center">
          No feeds added yet
        </div>
        <div v-else v-for="feed in feeds" :key="feed.id" class="p-4">
          <!-- Normal View -->
          <div v-if="editingFeedId !== feed.id" class="flex justify-between items-center">
            <div class="flex-1">
              <h3 class="font-medium">{{ feed.title }}</h3>
              <p class="text-sm text-gray-400">{{ feed.url }}</p>
            </div>
            <div class="flex items-center gap-3">
              <Switch
                :model-value="feed.enabled"
                @update:modelValue="() => toggleFeedEnabled(feed.id)"
              />
              <Button
                variant="ghost"
                size="icon"
                @click="startEditing(feed.id)"
              >
                <Edit2 class="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="deleteFeed(feed.id)"
                :disabled="isSubmitting"
              >
                <Trash2 class="h-4 w-4 text-red-500" />
              </Button>
            </div>
          </div>

          <!-- Edit View -->
          <div v-else class="space-y-3">
            <div class="grid gap-2">
              <Label for="edit-url">Feed URL</Label>
              <Input id="edit-url" v-model="editFeedUrl" />
            </div>
            <div class="grid gap-2">
              <Label for="edit-title">Feed Title</Label>
              <Input id="edit-title" v-model="editFeedTitle" />
            </div>
            <div class="flex items-center space-x-2">
              <Switch id="edit-enabled" v-model="editFeedEnabled" />
              <Label for="edit-enabled">Enable feed</Label>
            </div>
            <div class="flex items-center space-x-2">
              <Switch
                id="edit-dynamic-scraping"
                :model-value="editFeedDynamicScraping"
                @update:modelValue="setEditScraping(editFeedDynamicScraping ? null : 'dynamic')"
              />
              <Label for="edit-dynamic-scraping">Playwright (dynamic scraping)</Label>
              <div class="ml-2 text-xs text-gray-400">For JavaScript-heavy sites</div>
            </div>

            <div class="flex items-center space-x-2">
              <Switch
                id="edit-full-browser"
                :model-value="editFeedFullBrowser"
                @update:modelValue="setEditScraping(editFeedFullBrowser ? null : 'full_browser')"
              />
              <Label for="edit-full-browser">Full Browser (Solimen)</Label>
              <div class="ml-2 text-xs text-gray-400">Full browser rendering via Solimen</div>
            </div>

            <template v-if="editFeedFullBrowser">
              <h3 class="text-sm font-medium mt-2">Browser Triggers</h3>
              <div class="grid gap-2">
                <Label for="edit-triggers-loaded">Loaded Selectors</Label>
                <Input id="edit-triggers-loaded" v-model="editTriggersLoaded" placeholder=".article-body, #content" />
                <p class="text-xs text-gray-400">Comma-separated CSS selectors — all must match for the page to be considered loaded</p>
              </div>
              <div class="grid gap-2">
                <Label for="edit-triggers-failed">Failed Selectors</Label>
                <Input id="edit-triggers-failed" v-model="editTriggersFailed" placeholder=".error-page, #captcha" />
                <p class="text-xs text-gray-400">Comma-separated CSS selectors — all must match to indicate a load failure</p>
              </div>
            </template>

            <h3 class="text-sm font-medium mt-4">Content Extraction Selectors</h3>
            <div class="grid gap-2">
              <Label for="edit-article-selector">Article Selector</Label>
              <Input id="edit-article-selector" v-model="editArticleSelector" placeholder="article, .article-content, #main-content" />
              <p class="text-xs text-gray-400">CSS selector to find the article content</p>
            </div>
            
            <div class="grid gap-2">
              <Label for="edit-cutoff-selector">Cutoff Selector</Label>
              <Input id="edit-cutoff-selector" v-model="editCutoffSelector" placeholder=".comments, .social-icons" />
              <p class="text-xs text-gray-400">CSS selector to mark where to cutoff the article</p>
            </div>
            
            <div class="grid gap-2">
              <Label for="edit-blacklist-selector">Blacklist Selector</Label>
              <Input id="edit-blacklist-selector" v-model="editBlacklistSelector" placeholder="script, style, nav, .ads" />
              <p class="text-xs text-gray-400">Elements to exclude from the article</p>
            </div>
            <div class="flex justify-end gap-2">
              <Button variant="outline" @click="cancelEditing">
                <X class="h-4 w-4 mr-1" />
                Cancel
              </Button>
              <Button @click="saveEditedFeed(feed.id)" :disabled="isSubmitting">
                <Save class="h-4 w-4 mr-1" />
                Save
              </Button>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>