<script setup lang="ts">
import { TagIcon } from 'lucide-vue-next';
import { models } from "../../../wailsjs/go/models.ts";
import { Badge } from '@/components/ui/badge';

defineProps<{
  article: models.Article;
}>();
</script>

<template>
  <div v-if="article.tags && article.tags.length > 0" class="inline-flex gap-1 ml-2 max-w-full flex-wrap">
    <Badge
      v-for="tag in article.tags"
      :key="tag.id"
      variant="secondary"
      class="flex items-center gap-1 text-xs py-2"
      :style="tag.color ? `background-color: ${tag.color}30; color: ${tag.color}` : ''"
    >
      <TagIcon class="h-3 w-3" />
      {{ tag.name.replace("#", "") }}
    </Badge>
  </div>
  <div
    v-else
    class="inline-flex gap-1 ml-2 max-w-full flex-wrap"
  >
    <span class="italic text-xs">No tags</span>
  </div>
</template>