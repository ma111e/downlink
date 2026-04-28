<script setup lang="ts">
import {
  BookmarkCheckIcon,
  BookmarkIcon,
  BookOpenCheckIcon,
  BookOpenIcon,
  MoreVerticalIcon,
  ShareIcon,
  ExternalLinkIcon
} from 'lucide-vue-next';
import {onBeforeUnmount, onMounted, ref} from 'vue';
import {models} from "../../../wailsjs/go/models.ts";
import {useArticles} from '@/composables/useArticles.ts';
import {useToast} from "@/components/ui/toast";
import {Button} from "@/components/ui/button"
import { BrowserOpenURL } from "../../../wailsjs/runtime";

const props = defineProps<{
  article: models.Article;
}>();

const emit = defineEmits<{
  (e: 'articleUpdated', article: models.Article): void;
}>();

const menuOpen = ref(false);
const menuButtonRef = ref<HTMLButtonElement | null>(null);
const {updateArticle} = useArticles();
const {toast} = useToast();

// Toggle bookmark status
const toggleBookmark = async () => {
  try {
    const updatedArticle = await updateArticle(props.article.id, {
      bookmarked: !props.article.bookmarked,
    });

    if (!updatedArticle) {
      throw new Error('Article update did not return a cached article');
    }

    emit('articleUpdated', updatedArticle);

    toast({
      title: updatedArticle.bookmarked ? "Bookmarked" : "Bookmark removed",
      description: updatedArticle.bookmarked ? "Article added to bookmarks" : "Article removed from bookmarks",
      duration: 3000,
    });
  } catch (error) {
    console.error("Error toggling bookmark:", error);
    toast({
      title: "Error",
      description: "Could not update bookmark status",
      variant: "destructive",
      duration: 3000,
    });
  }
};

// Toggle read status
const toggleReadStatus = async () => {
  try {
    const updatedArticle = await updateArticle(props.article.id, {
      read: !props.article.read,
    });

    if (!updatedArticle) {
      throw new Error('Article update did not return a cached article');
    }

    emit('articleUpdated', updatedArticle);

    toast({
      title: updatedArticle.read ? "Marked as read" : "Marked as unread",
      description: updatedArticle.read ? "Article marked as read" : "Article marked as unread",
      duration: 3000,
    });
  } catch (error) {
    console.error("Error toggling read status:", error);
    toast({
      title: "Error",
      description: "Could not update read status",
      variant: "destructive",
      duration: 3000,
    });
  }
};

// Share article
const shareArticle = async () => {
  if (!props.article.link) {
    toast({
      title: "Cannot share",
      description: "This article has no URL to share",
      variant: "destructive",
      duration: 3000
    });
    return;
  }

  try {
    await navigator.clipboard.writeText(props.article.link);
    toast({
      title: "URL copied to clipboard",
      description: "The article URL has been copied to your clipboard",
      duration: 3000
    });
  } catch (err) {
    toast({
      title: "Error",
      description: "Could not copy URL to clipboard",
      variant: "destructive",
      duration: 3000
    });
  }
};

// Open article in browser
const openInBrowser = () => {
  if (!props.article.link) {
    toast({
      title: "Cannot open",
      description: "This article has no URL to open",
      variant: "destructive",
      duration: 3000
    });
    return;
  }

  BrowserOpenURL(props.article.link);
};

// Toggle menu
const toggleMenu = () => {
  menuOpen.value = !menuOpen.value;
};

// Handle outside click to close menu
const handleOutsideClick = (event: MouseEvent) => {
  if (menuOpen.value && menuButtonRef.value && !menuButtonRef.value.contains(event.target as Node)) {
    menuOpen.value = false;
  }
};

// Add/remove event listeners
onMounted(() => {
  document.addEventListener('click', handleOutsideClick);
});

onBeforeUnmount(() => {
  document.removeEventListener('click', handleOutsideClick);
});
</script>

<template>
  <div class="items-center gap-1 ">
    <div
        class="relative hidden md:flex"
    >

      <!-- Action Buttons -->
      <Button
          variant="ghost"
          @click="toggleBookmark"
          class="text-gray-400 hover:text-white p-2"
          :aria-label="article.bookmarked ? 'Remove bookmark' : 'Add bookmark'"
          :title="article.bookmarked ? 'Remove bookmark' : 'Add bookmark'"
      >
        <BookmarkIcon v-if="!article.bookmarked" class="h-5 w-5"/>
        <BookmarkCheckIcon v-else class="h-5 w-5 text-amber-400"/>
      </Button>

      <Button
          variant="ghost"
          @click="toggleReadStatus"
          class="text-gray-400 hover:text-white p-2"
          :aria-label="article.read ? 'Mark as unread' : 'Mark as read'"
          :title="article.read ? 'Mark as unread' : 'Mark as read'"
      >
        <BookOpenIcon v-if="!article.read" class="h-5 w-5"/>
        <BookOpenCheckIcon v-else class="h-5 w-5 text-emerald-400"/>
      </Button>

      <Button
          variant="ghost"
          @click="shareArticle"
          class="text-gray-400 hover:text-white p-2"
          aria-label="Share article"
          title="Share article"
      >
        <ShareIcon class="h-5 w-5"/>
      </Button>

      <Button
          variant="ghost"
          @click="openInBrowser"
          class="text-gray-400 hover:text-white p-2"
          aria-label="Open in browser"
          title="Open in browser"
      >
        <ExternalLinkIcon class="h-5 w-5"/>
      </Button>
    </div>

    <!-- Mobile Menu Button -->
    <div
        class="relative md:hidden"
    >
      <Button
          variant="ghost"
          ref="menuButtonRef"
          @click="toggleMenu"
          class="text-gray-400 hover:text-white p-2"
          aria-label="More actions"
          title="More actions"
      >
        <MoreVerticalIcon class="h-5 w-5"/>
      </Button>

      <!-- Dropdown Menu -->
      <div
          v-if="menuOpen"
          class="absolute right-0 mt-2 w-48 rounded-md shadow-lg bg-gray-800 ring-1 ring-black ring-opacity-5 z-10"
      >
        <div class="py-1" role="menu" aria-orientation="vertical" aria-labelledby="options-menu">
          <Button
              variant="ghost"
              @click="toggleBookmark"
              class="block w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-gray-700"
              role="menuitem"
          >
            <div class="flex items-center">
              <BookmarkIcon v-if="!article.bookmarked" class="h-4 w-4 mr-2"/>
              <BookmarkCheckIcon v-else class="h-4 w-4 mr-2 text-amber-400"/>
              {{ article.bookmarked ? 'Remove bookmark' : 'Add bookmark' }}
            </div>
          </Button>

          <Button
              variant="ghost"
              @click="toggleReadStatus"
              class="block w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-gray-700"
              role="menuitem"
          >
            <div class="flex items-center">
              <BookOpenIcon v-if="!article.read" class="h-4 w-4 mr-2"/>
              <BookOpenCheckIcon v-else class="h-4 w-4 mr-2 text-emerald-400"/>
              {{ article.read ? 'Mark as unread' : 'Mark as read' }}
            </div>
          </Button>

          <Button
              variant="ghost"
              @click="shareArticle"
              class="block w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-gray-700"
              role="menuitem"
          >
            <div class="flex items-center">
              <ShareIcon class="h-4 w-4 mr-2"/>
              Share article
            </div>
          </Button>

          <Button
              variant="ghost"
              @click="openInBrowser"
              class="block w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-gray-700"
              role="menuitem"
          >
            <div class="flex items-center">
              <ExternalLinkIcon class="h-4 w-4 mr-2"/>
              Open in browser
            </div>
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
button {
  transition: all 0.2s;
}
</style>
