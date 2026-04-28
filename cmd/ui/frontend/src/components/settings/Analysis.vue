<script setup lang="ts">
import {inject, onMounted, ref} from 'vue';
import {UpdateAnalysisConfig} from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import {models} from "../../../wailsjs/go/models.ts";
import {useToast} from "@/components/ui/toast";
import {Textarea} from "@/components/ui/textarea";
import {LoaderIcon} from "lucide-vue-next";
import {useArticleAnalysisStore} from "@/stores/articleAnalysisStore.ts";
import ProviderConfig = models.ProviderConfig;

const articleAnalysisStore = useArticleAnalysisStore();

const {
  loadLLMProviders,
  loadAnalysisConfig,
} = articleAnalysisStore;

const {toast} = useToast();
const isLoading = ref(true);

// Original values for change detection
const originalProvider = ref('');
const originalPersona = ref('');

// Get references to global state and actions
const setHasUnsavedChanges = inject('setHasUnsavedChanges') as (value: boolean) => void;
const registerSaveCallback = inject('registerSaveCallback') as (callback: () => Promise<void>) => void;

// Load digest configuration and LLM providers when the component mounts
onMounted(async () => {
  try {
    isLoading.value = true;
    await Promise.all([loadAnalysisConfig(), loadLLMProviders()]);

    // Store original state for change detection
    originalProvider.value = articleAnalysisStore.selectedProvider;
    originalPersona.value = articleAnalysisStore.persona;

    // Watch for changes to update the global unsaved changes flag
    watchForChanges();

    // Register our save function with the parent component
    registerSaveCallback(saveSettings);
  } catch (err) {
    console.error('Failed to load analysis settings:', err);
    toast({
      title: "Error",
      description: `Failed to load analysis settings: ${err}`,
      variant: "destructive"
    });
  } finally {
    isLoading.value = false;
  }
});

// Handle provider selection
const onProviderChange = (provider: ProviderConfig) => {
  if (!provider) return;
  articleAnalysisStore.selectedProvider = provider.name || '';
};

// Save configuration
const saveSettings = async () => {
  try {
    const analysisConfig = models.AnalysisConfig.createFrom({
      ...articleAnalysisStore.analysisConfig,
      provider: articleAnalysisStore.selectedProvider,
      persona: articleAnalysisStore.persona,
    });

    await UpdateAnalysisConfig(analysisConfig);

    // Refresh configs from backend
    await Promise.all([loadAnalysisConfig(), loadLLMProviders()]);

    // Update original state
    originalProvider.value = articleAnalysisStore.selectedProvider;
    originalPersona.value = articleAnalysisStore.persona;

    setHasUnsavedChanges(false);

    toast({
      title: "Success",
      description: "Analysis settings saved successfully!",
      variant: "default",
      duration: 1000
    });

    return Promise.resolve();
  } catch (error) {
    console.error('Failed to save analysis settings:', error);
    return Promise.reject(error);
  }
};

// Watch for changes to update the global unsaved changes flag
const watchForChanges = () => {
  setInterval(() => {
    const hasChanges =
        articleAnalysisStore.selectedProvider !== originalProvider.value ||
        articleAnalysisStore.persona !== originalPersona.value;

    setHasUnsavedChanges(hasChanges);
  }, 300);
};

</script>

<template>
  <!-- Loading state -->
  <div v-if="isLoading" class="text-center py-8">
    <LoaderIcon class="h-12 w-12 mb-4 mx-auto animate-spin"/>
    <p>Loading analysis settings...</p>
  </div>


  <!-- LLM Providers Section -->
  <section class="mb-8">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-lg font-medium">Analysis Configuration</h2>
    </div>

    <div class="max-w-2xl mx-auto">
      <!-- Provider selection -->
      <div class="space-y-4 p-4 rounded-lg bg-gray-900">
        <div v-if="articleAnalysisStore.providers.filter((p: ProviderConfig) => p.enabled).length === 0"
             class="text-amber-500 text-sm py-2">
          No enabled LLM providers found. Please configure and enable at least one provider in the <strong>LLM</strong>
          settings.
        </div>

        <div v-else class="space-y-5">
          <div class="space-y-2">
            <label class="block font-medium">Default Provider</label>
            <p class="text-xs text-gray-400">Choose from your enabled LLM providers for article analysis</p>

            <div class="space-y-3 mt-2">
              <div
                  v-for="provider in articleAnalysisStore.providers.filter((p: ProviderConfig) => p.enabled)"
                  :key="provider.name || `${provider.provider_type}-${provider.model_name}`"
                  class="flex items-center gap-3 p-3 border rounded-md transition-colors cursor-pointer"
                  :class="{
                  'border-primary bg-primary/10': articleAnalysisStore.selectedProvider === provider.name,
                  'border-gray-700 bg-gray-800 hover:bg-gray-700': articleAnalysisStore.selectedProvider !== provider.name
                }"
                  @click="onProviderChange(provider)"
              >
                <div class="flex-1">
                  <div class="font-medium">
                    {{ provider.name || `${provider.provider_type}/${provider.model_name || 'No model'}` }}
                  </div>
                  <div class="text-xs text-gray-400 mt-1">
                    {{ provider.provider_type }}/{{ provider.model_name || 'No model' }}
                    {{ provider.temperature != null ? ` • Temperature: ${provider.temperature}` : '' }}
                    {{ provider.max_retries != null ? ` • Max retries: ${provider.max_retries}` : '' }}
                    {{ provider.timeout_minutes != null ? ` • Timeout: ${provider.timeout_minutes}min` : '' }}
                  </div>
                </div>
              </div>
            </div>

            <p v-if="articleAnalysisStore.providers.filter((p: ProviderConfig) => p.enabled && !p.name).length > 0"
               class="text-xs text-amber-500 mt-2">
              Some providers have no name set. Give each provider a name in the <strong>LLM</strong> tab so they can be selected here.
            </p>
            <p v-else class="text-xs text-amber-500 mt-2">
              You can configure more providers and their options in the <strong>LLM</strong> tab.
            </p>
          </div>
        </div>
      </div>


      <!-- Persona Section -->
      <div class="space-y-4 p-4 rounded-lg bg-gray-900 mb-6">
        <h3 class="font-medium">Persona</h3>
        <div class="grid gap-2">
          <Textarea
              id="prompt-prefix"
              v-model="articleAnalysisStore.persona"
              rows="10"
              placeholder="You are an expert cyber threat analyst..."
              class="bg-gray-800 border-gray-700 text-white"
          />
          <p class="text-xs text-gray-400 mt-1">
            This text will be prepended to prompts sent to the AI. Use it to provide context and instructions for the
            model.
          </p>
        </div>
      </div>

    </div>
  </section>

</template>
