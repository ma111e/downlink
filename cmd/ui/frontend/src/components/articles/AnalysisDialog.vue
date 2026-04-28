<script setup lang="ts">
import { watch, onMounted } from 'vue';
import { useToast } from "@/components/ui/toast";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { useArticleAnalysisStore } from '@/stores/articleAnalysisStore';
import { useAnalysisQueueStore } from '@/stores/analysisQueueStore';
import { LoaderIcon } from "lucide-vue-next";
import {models} from "../../../wailsjs/go/models.ts";
import ProviderConfig = models.ProviderConfig;

const props = defineProps<{
  articleId: string;
  open: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void;
  (e: 'analysisStarted'): void;
}>();

const { toast } = useToast();

// Use the Pinia stores
const articleAnalysisStore = useArticleAnalysisStore();
const queueStore = useAnalysisQueueStore();
const { loadLLMProviders } = articleAnalysisStore;

// Enqueue article with selected provider and model, auto-start if idle
const analyzeArticle = async () => {
  if (!props.articleId || !articleAnalysisStore.selectedProviderType || !articleAnalysisStore.selectedModelName) {
    toast({
      title: "Missing information",
      description: "Please select a provider",
      variant: "destructive",
      duration: 3000
    });
    return;
  }

  queueStore.resetProgress(props.articleId);
  emit('analysisStarted');
  emit('update:open', false);

  await queueStore.enqueueOne(
    props.articleId,
    articleAnalysisStore.selectedProviderType,
    articleAnalysisStore.selectedModelName
  );
};

// Load providers when dialog is opened
watch(() => props.open, (newValue) => {
  if (newValue) {
    loadLLMProviders();
  }
});

// Load providers when the component is mounted if dialog is open
onMounted(() => {
  if (props.open) {
    loadLLMProviders();
  }
});
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-w-md bg-gray-900 border-gray-800 text-white">
      <DialogHeader>
        <DialogTitle>Analyze Article</DialogTitle>
        <DialogDescription class="text-gray-400">
          Select a provider and model to analyze this article
        </DialogDescription>
      </DialogHeader>

      <div class="space-y-4 my-2">
        <!-- Provider Selection -->
        <div class="space-y-2">
          <label class="text-sm font-medium text-gray-300">Provider</label>
          <div class="relative">
            <div v-if="articleAnalysisStore.isLoadingProviders" class="flex items-center justify-center p-2">
              <LoaderIcon class="h-4 w-4 mr-2 animate-spin" />
              Loading providers...
            </div>
            
            <div v-else-if="articleAnalysisStore.providers.filter((p: ProviderConfig) => p.enabled).length === 0" class="text-amber-500 text-sm py-2">
              No enabled LLM providers found. Please configure and enable at least one provider in the settings.
            </div>

            <div v-else class="space-y-3 mt-2">
              <div
                v-for="provider in articleAnalysisStore.providers.filter((p: ProviderConfig) => p.enabled)"
                :key="`${provider.provider_type}-${provider.model_name}`"
                class="flex items-center gap-3 p-3 border rounded-md transition-colors cursor-pointer"
                :class="{
                  'border-primary bg-primary/10': articleAnalysisStore.selectedProviderType === provider.provider_type && articleAnalysisStore.selectedModelName === provider.model_name,
                  'border-gray-700 bg-gray-800 hover:bg-gray-700': !(articleAnalysisStore.selectedProviderType === provider.provider_type && articleAnalysisStore.selectedModelName === provider.model_name)
                }"
                @click="() => {
                  articleAnalysisStore.selectedProviderType = provider.provider_type;
                  articleAnalysisStore.selectedModelName = provider.model_name;
                }"
              >
                <div class="flex-1">
                  <div class="font-medium">{{ `${provider.provider_type}/${provider.model_name || 'No model'}` }}</div>
                  <div class="text-xs text-gray-400 mt-1">
                    {{ provider.temperature ? `Temperature: ${provider.temperature}` : '' }}
                    {{ provider.max_retries ? ` • Max retries: ${provider.max_retries}` : '' }}
                    {{ provider.timeout_minutes ? ` • Timeout: ${provider.timeout_minutes}min` : '' }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- We don't need a separate model selection as it's now part of the provider card selection -->
      </div>

      <DialogFooter>
        <Button 
          variant="ghost" 
          @click="emit('update:open', false)"
        >
          Cancel
        </Button>
        <Button 
          :disabled="!articleAnalysisStore.selectedProviderType || !articleAnalysisStore.selectedModelName || articleAnalysisStore.providers.filter((p: ProviderConfig) => p.enabled).length === 0"
          @click="analyzeArticle"
        >
          Analyze
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
