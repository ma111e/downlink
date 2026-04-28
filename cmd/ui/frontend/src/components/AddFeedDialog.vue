<script setup lang="ts">
import { ref } from 'vue';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { GetConfig, SaveConfig } from "../../wailsjs/go/downlinkclient/DownlinkClient";
import { useFeeds } from '@/composables/useFeeds';
import {models} from "../../wailsjs/go/models.ts";
import FeedConfig = models.FeedConfig;

const props = defineProps<{
  open: boolean;
}>();

const emit = defineEmits(['update:open']);

const { fetchFeeds } = useFeeds();

// Form data
const feedUrl = ref('');
const feedTitle = ref('');
const feedType = ref('rss');
const feedEnabled = ref(true);
const feedDynamicScraping = ref(false);
const feedFullBrowser = ref(false);

// Selectors
const articleSelector = ref('');
const cutoffSelector = ref('');
const blacklistSelector = ref('');

const isSubmitting = ref(false);
const errorMessage = ref('');

// Close dialog
const closeDialog = () => {
  emit('update:open', false);
  resetForm();
};

// Reset form fields
const resetForm = () => {
  feedUrl.value = '';
  feedTitle.value = '';
  feedType.value = 'rss';
  feedEnabled.value = true;
  feedDynamicScraping.value = false;
  feedFullBrowser.value = false;
  
  articleSelector.value = '';
  cutoffSelector.value = '';
  blacklistSelector.value = '';
  
  errorMessage.value = '';
};

// Submit form
const handleSubmit = async () => {
  if (!feedUrl.value) {
    errorMessage.value = 'Feed URL is required';
    return;
  }

  try {
    isSubmitting.value = true;
    errorMessage.value = '';

    // Get current models
    const config = await GetConfig();

    // Create feed models object
    const scraper: Record<string, any> = {};
    if (feedFullBrowser.value) {
      scraper['scraping'] = 'full_browser';
    } else if (feedDynamicScraping.value) {
      scraper['dynamic_scraping'] = 'true';
    }
    if (articleSelector.value) {
      scraper['article'] = articleSelector.value;
    }
    if (cutoffSelector.value) {
      scraper['cutoff'] = cutoffSelector.value;
    }
    if (blacklistSelector.value) {
      scraper['blacklist'] = blacklistSelector.value;
    }

    const newFeed = <FeedConfig>{
      url: feedUrl.value,
      type: feedType.value,
      title: feedTitle.value,
      enabled: feedEnabled.value,
      scraper: Object.keys(scraper).length > 0 ? scraper : undefined,
    };

    // Add new feed to models
    config.feeds.push(newFeed);

    // Save updated models
    await SaveConfig(config);

    // Refresh feeds list to reflect changes
    await fetchFeeds();

    // Close dialog and reset form
    closeDialog();
  } catch (error) {
    console.error('Failed to add feed:', error);
    errorMessage.value = `Failed to add feed: ${error}`;
  } finally {
    isSubmitting.value = false;
  }
};
</script>

<template>
  <Dialog :open="props.open" @update:open="emit('update:open', $event)">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Add New Feed</DialogTitle>
        <DialogDescription>
          Enter the details of the feed you want to add.
        </DialogDescription>
      </DialogHeader>

      <div class="grid gap-4 py-4">
        <div class="grid gap-2">
          <Label for="url">Feed URL</Label>
          <Input
            id="url"
            v-model="feedUrl"
            required
          />
        </div>

        <div class="grid gap-2">
          <Label for="title">Feed Title</Label>
          <Input
            id="title"
            v-model="feedTitle"
          />
        </div>

        <div class="flex items-center space-x-2">
          <Switch id="enabled" v-model="feedEnabled" />
          <Label for="enabled">Enable feed</Label>
        </div>
        
        <div class="flex items-center space-x-2">
          <Switch id="dynamicScraping" v-model="feedDynamicScraping" />
          <Label for="dynamicScraping">Use dynamic scraping (Playwright)</Label>
        </div>

        <div class="flex items-center space-x-2">
          <Switch id="fullBrowser" v-model="feedFullBrowser" />
          <Label for="fullBrowser">Use full browser (Solimen)</Label>
        </div>
        
        <h3 class="text-sm font-medium mt-4">Content Extraction Selectors</h3>
        <div class="grid gap-2">
          <Label for="articleSelector">Article Selector</Label>
          <Input
            id="articleSelector"
            v-model="articleSelector"
            placeholder="article, .article-content, #main-content"
          />
          <p class="text-xs text-gray-400">CSS selector to find the article content</p>
        </div>
        
        <div class="grid gap-2">
          <Label for="cutoffSelector">Cutoff Selector</Label>
          <Input
            id="cutoffSelector"
            v-model="cutoffSelector"
            placeholder=".comments, .related-articles"
          />
          <p class="text-xs text-gray-400">CSS selector to mark where to cutoff the article</p>
        </div>
        
        <div class="grid gap-2">
          <Label for="blacklistSelector">Blacklist Selector</Label>
          <Input
            id="blacklistSelector"
            v-model="blacklistSelector"
            placeholder="script, style, nav, .ads"
          />
          <p class="text-xs text-gray-400">Elements to exclude from the article</p>
        </div>

        <div v-if="errorMessage" class="text-red-500 text-sm">
          {{ errorMessage }}
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="closeDialog">Cancel</Button>
        <Button type="submit" @click="handleSubmit" :disabled="isSubmitting">
          <span v-if="isSubmitting">Adding...</span>
          <span v-else>Add Feed</span>
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>