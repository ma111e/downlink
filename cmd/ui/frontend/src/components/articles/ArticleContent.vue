<script setup lang="ts">
import { models } from "../../../wailsjs/go/models.ts";
import { ref, watch, nextTick } from 'vue';
import { marked } from 'marked';
import { BrowserOpenURL } from "../../../wailsjs/runtime";
import DOMPurify from 'dompurify';

// Image Viewer State
const imageViewerSrc = ref<string>('');
const isImageViewerOpen = ref<boolean>(false);

// Hovered link URL
const hoveredLink = ref<string>('');

const articleDisplay = ref<string>('');

const props = defineProps<{
  article: models.Article;
}>();

watch(() => props.article, async (newArticle: models.Article) => {
  if (!newArticle.content) {
    articleDisplay.value = "<i>No data</i>";
    return
  };

  nextTick(() => {
    removeInlineStyles();
    preventLinkNavigation();
    attachLinkHandlers();
  });

  if (newArticle.content.includes('<div')) {  // at least one div
    articleDisplay.value = newArticle.content;
    return;     
  };

  const clean = DOMPurify.sanitize(newArticle.content);
  articleDisplay.value = await marked.parse(clean);
}, { immediate: true });

const removeInlineStyles = () => {
  const article = document.querySelector('.prose');
  if (!article) return;

  // Remove width styles from divs with wp-caption class
  const captions = article.querySelectorAll('.wp-caption');
  captions.forEach(caption => {
    if (caption instanceof HTMLElement) {
      caption.style.height = '';
      caption.style.width = '';
      caption.style.maxWidth = '100%';
    }
  });

  // Remove width styles from images
  const images = article.querySelectorAll('img');
  images.forEach(img => {
    if (img instanceof HTMLElement) {
      img.style.width = '';
      img.style.maxWidth = '100%';
      img.style.height = '';
    }
  });
};

const preventLinkNavigation = () => {
  const article = document.querySelector('.prose');
  if (!article) return;

  // Prevent navigation for links containing images
  const links = article.querySelectorAll('a');
  links.forEach(link => {
    link.addEventListener('click', (event) => {
      // Check if the clicked target or its parent is an image
      const clickedImage = event.target instanceof HTMLImageElement || 
                           (event.target as HTMLElement).querySelector('img');
      
      if (clickedImage) {
        event.preventDefault();
        event.stopPropagation();
      }
    });
  });
};
const attachLinkHandlers = () => {
  const article = document.querySelector('.prose');
  if (!article) return;

  article.querySelectorAll('img').forEach(img => {
    img.addEventListener('click', (event) => {
      event.preventDefault();
      event.stopPropagation();
      imageViewerSrc.value = (event.currentTarget as HTMLImageElement).src || '';
      isImageViewerOpen.value = true;
    });
  });

  const links = article.querySelectorAll('a');
  links.forEach(link => {
    link.addEventListener('mouseenter', () => {
      hoveredLink.value = (link as HTMLAnchorElement).href || '';
    });
    link.addEventListener('mouseleave', () => {
      hoveredLink.value = '';
    });
    link.addEventListener('click', (event) => {
      event.preventDefault();
      event.stopPropagation();

      // Check if it's an image link
      const clickedImage = event.target instanceof HTMLImageElement || 
                           (event.target as HTMLElement).querySelector('img');
      
      if (clickedImage) {
        // Open image viewer for image links
        imageViewerSrc.value = (event.currentTarget as HTMLAnchorElement).href || '';
        isImageViewerOpen.value = true;
      } else {
        // Use Wails BrowserOpenURL for external links
        const href = (event.currentTarget as HTMLAnchorElement).href;
        if (href) {
          BrowserOpenURL(href);
        }
      }
    });
  });
};
const closeImageViewer = () => {
  isImageViewerOpen.value = false;
  imageViewerSrc.value = '';
};
</script>

<template>
  <div class="max-w-2xl w-full">
    <div v-html="articleDisplay" class="prose max-w-2xl overflow-y-auto"></div>

    <!-- Link URL status bar -->
    <div
      v-if="hoveredLink"
      class="fixed bottom-0 left-0 right-0 z-40 px-3 py-1 text-xs text-gray-300 bg-gray-900 border-t border-gray-700 truncate"
    >{{ hoveredLink }}</div>
    
    <!-- Image Viewer Modal -->
    <div 
      v-if="isImageViewerOpen" 
      class="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-80"
      @click="closeImageViewer"
    >
      <div 
        class="max-w-[90%] max-h-[90%] flex items-center justify-center" 
        @click.stop
      >
        <img 
          :src="imageViewerSrc" 
          alt="Full Image" 
          class="max-w-full max-h-full object-contain shadow-2xl rounded-lg"
        />
        <button 
          @click="closeImageViewer" 
          class="absolute top-4 right-4 text-white text-2xl hover:text-gray-300"
        >
          ✕
        </button>
      </div>
    </div>
  </div>
</template>

<style>
.prose {
  color: rgba(255, 255, 255, 0.9);
}

.prose a {
  color: #60a5fa;
  text-decoration: underline;
}

.prose a:hover {
  color: #93c5fd;
}

.prose p{
  margin-top: 16px; 
  margin-bottom: 16px;
  text-align: justify;
}

.wp-caption{
  background-color: rgba(17, 24, 39, 0.8);
  border-radius: 0.375rem;
  padding: 0.75rem;
  text-align: center;
}

.prose img {
  cursor: pointer; /* Add pointer cursor to indicate clickable images */
}

.prose ul, 
.prose ol {
  padding-left: 1.5rem;
  margin: 1rem 0;
}

.prose ul {
  list-style-type: disc;
}

.prose ol {
  list-style-type: decimal;
}

.prose li {
  margin-bottom: 0.5rem;
  color: rgba(255, 255, 255, 0.85);
}

.prose ul ul,
.prose ol ul,
.prose ul ol,
.prose ol ol {
  margin: 0.5rem 0;
}

.prose pre {
  background-color: rgba(17, 24, 39, 0.8);
  border-radius: 0.375rem;
  padding: 0.75rem;
  overflow-x: auto;
}

.prose code {
  background-color: rgba(17, 24, 39, 0.5);
  border-radius: 0.25rem;
  padding: 0.125rem 0.25rem;
  font-size: 0.875em;
}

.prose blockquote {
  border-left: 3px solid rgba(75, 85, 99, 0.8);
  padding-left: 1rem;
  font-style: italic;
  margin-left: 0;
  color: rgba(209, 213, 219, 0.8);
}

.prose h1, 
.prose h2, 
.prose h3, 
.prose h4, 
.prose h5, 
.prose h6 {
  color: white;
  margin-top: 1.5em;
  margin-bottom: 0.75em;
}
</style>