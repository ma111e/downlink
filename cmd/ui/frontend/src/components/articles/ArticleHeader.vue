<script setup lang="ts">
import { ExternalLinkIcon } from 'lucide-vue-next';
import { models } from "../../../wailsjs/go/models.ts";
import { useFormatters } from '@/composables/useFormatters.ts';
import { BrowserOpenURL } from "../../../wailsjs/runtime";

const props = defineProps<{
  article: models.Article;
  feedTitle: string;
}>();

const { formatDate } = useFormatters();

// Function to open article in web browser
const openInWeb = () => {
  if (props.article.link) {
    BrowserOpenURL(props.article.link);
  }
};
</script>

<template>
  <div
  class="bg-gray-900"
  >
    <!-- Article title and link -->
    <div class=" p-4 py-2 flex justify-between items-center relative">
      <h2
        class="text-xl font-bold leading-tight line-clamp-2 cursor-pointer hover:underline"
        @click="openInWeb"
        :title="article.link ? 'Open in browser' : ''"
      >
        {{ article.title }}
        <ExternalLinkIcon v-if="article.link" class="h-4 w-4 inline ml-1" />
      </h2>
    </div>

    <!-- Article metadata: feed and date -->
    <div class="px-4 text-gray-400 text-sm flex justify-between">
      <div>
        <span>{{ feedTitle }}</span>
      </div>
      <span>{{ formatDate(article.published_at) }}</span>
    </div>
  </div>
</template>

<style scoped>
h2 {
  transition: color 0.2s;
}
</style>
