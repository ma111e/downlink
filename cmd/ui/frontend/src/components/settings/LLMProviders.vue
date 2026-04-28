<script setup lang="ts">
import {onMounted, ref, computed} from 'vue';
import {useArticleAnalysisStore} from '@/stores/articleAnalysisStore.ts';
import {Label} from '@/components/ui/label';
import {Input} from '@/components/ui/input';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue,} from "@/components/ui/select";
import {Accordion, AccordionContent, AccordionItem, AccordionTrigger} from "@/components/ui/accordion";
import {useConfig} from '@/composables/useConfig';
import {GetAvailableModelsForProvider, GetLLMProviders, SaveLLMProviders, TestProviderConnection} from "../../../wailsjs/go/downlinkclient/DownlinkClient";
import {models, downlinkclient} from "../../../wailsjs/go/models.ts";
import {useToast} from "@/components/ui/toast";
import ProviderConfig = models.ProviderConfig;
import ModelInfo = models.ModelInfo;
import ConnectionTestResult = downlinkclient.ConnectionTestResult;
import type {AcceptableValue} from "reka-ui";
import { LoaderIcon, CheckCircleIcon, XCircleIcon } from "lucide-vue-next";

// Remove references to config and configLoading that aren't used
const {fetchConfig} = useConfig();
const {toast} = useToast(); // Add toast functionality

// Use the shared Pinia store for providers
const articleAnalysisStore = useArticleAnalysisStore();
const providers = computed({
  get: () => articleAnalysisStore.providers,
  set: (val) => { articleAnalysisStore.providers = val; }
});

// Computed sorted providers: enabled first
const sortedProviders = computed(() => {
  return [...providers.value].sort((a: ProviderConfig, b: ProviderConfig) => {
    return (b.enabled === true ? 1 : 0) - (a.enabled === true ? 1 : 0);
  });
});

const isSubmitting = ref(false);
// Add overall loading state for initial data fetching
const isLoading = ref(true);
const newProviderInput = ref(''); // Selected provider type
const availableProviders = ref(['openai', 'anthropic', 'ollama', 'mistral', 'llamacpp']);
const availableModels = ref<ModelInfo[]>([]);

// Per-provider loading state keyed by "providerType:baseURL"
const loadingModelsState = ref<Record<string, boolean>>({});

// Per-provider connectivity test state keyed by "providerType:baseURL"
const connectionTestState = ref<Record<string, { testing: boolean; result: ConnectionTestResult | null }>>({});

const providerKey = (provider: ProviderConfig) => `${provider.provider_type}:${provider.base_url}`;

const clearTestState = (provider: ProviderConfig) => {
  delete connectionTestState.value[providerKey(provider)];
};

const testConnection = async (provider: ProviderConfig) => {
  const key = providerKey(provider);
  connectionTestState.value[key] = { testing: true, result: null };
  try {
    const result = await TestProviderConnection(provider.provider_type, provider.base_url ?? '', provider.api_key ?? '');
    connectionTestState.value[key] = { testing: false, result };
  } catch (e) {
    connectionTestState.value[key] = { testing: false, result: { success: false, message: String(e), latency_ms: 0 } };
  }
};

// Track original state to detect changes
const originalProviders = ref<ProviderConfig[]>([]);

// Track if there are unsaved changes - but only show if not loading
const hasUnsavedChanges = computed(() => {
  // If still loading, don't report unsaved changes
  if (isLoading.value) return false;

  return JSON.stringify(providers.value) !== JSON.stringify(originalProviders.value);
});

// Remove a provider by unique fields (provider_type + base_url)
const removeProvider = (providerToRemove: ProviderConfig) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToRemove.provider_type && p.base_url === providerToRemove.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers.splice(idx, 1);
  }
};

// Add a new provider - now triggered by the Add button
const addProvider = () => {
  if (!newProviderInput.value) {
    toast({
      title: "Error",
      description: "Please select a provider type first",
      variant: "destructive"
    });
    return;
  }

  const providerType = newProviderInput.value.trim().toLowerCase();

  // Set default base URL for local providers
  const baseUrl = providerType === 'ollama' ? 'http://localhost:11434'
                : providerType === 'llamacpp' ? 'http://localhost:8080'
                : undefined;

  // Create default values for the new configuration fields
  const temperature = 0.3;
  const maxRetries = 3;
  const timeoutMinutes = 5;

  articleAnalysisStore.providers.push({
    name: '',
    provider_type: providerType,
    model_name: '',
    enabled: true,
    base_url: baseUrl,
    temperature: temperature,
    max_retries: maxRetries,
    timeout_minutes: timeoutMinutes,
    api_key: ''
  });

  // Reset the input
  newProviderInput.value = '';

  // Fetch models for the newly added provider
  fetchAvailableModels();
};

// Fetch models for a single provider concurrently — does not block other providers.
const fetchModelsForProvider = async (provider: ProviderConfig) => {
  const key = providerKey(provider);
  loadingModelsState.value[key] = true;
  try {
    const response = await GetAvailableModelsForProvider(provider.provider_type, provider.base_url ?? '');
    if (response?.models?.length) {
      // Remove stale models for this provider then add fresh ones
      availableModels.value = [
        ...availableModels.value.filter(m => !(m.provider_type === provider.provider_type && (provider.base_url ? true : true))),
        ...response.models,
      ];
    }
  } catch (error) {
    console.error(`Failed to fetch models for ${provider.provider_type}:`, error);
  } finally {
    loadingModelsState.value[key] = false;
  }
};

// Fire off per-provider model fetches in parallel for all enabled providers.
const fetchAvailableModels = (providerList?: ProviderConfig[]) => {
  const targets = (providerList ?? providers.value).filter(p => p.enabled);
  // Deduplicate by providerType+baseURL
  const seen = new Set<string>();
  for (const p of targets) {
    const key = providerKey(p);
    if (!seen.has(key)) {
      seen.add(key);
      fetchModelsForProvider(p);
    }
  }
};

// Store original state after loading
const storeOriginalState = () => {
  originalProviders.value = JSON.parse(JSON.stringify(providers.value));
};



// Load data when component mounts
onMounted(async () => {
  try {
    isLoading.value = true;
    await Promise.all([fetchConfig(), loadLLMProviders()]);
    storeOriginalState();
  } catch (err) {
    console.error('Failed to load configurations:', err);
    toast({
      title: "Error",
      description: `Failed to load configurations: ${err}`,
      variant: "destructive"
    });
  } finally {
    isLoading.value = false;
  }
  // Fire per-provider model fetches after UI is unblocked
  fetchAvailableModels();
});

// Load LLM providers from backend
const loadLLMProviders = async () => {
  try {
    const backendProviders = await GetLLMProviders();

    if (backendProviders && backendProviders.length > 0) {
      articleAnalysisStore.providers = backendProviders;
    } else {
      articleAnalysisStore.providers = [];
    }
  } catch (error) {
    console.error('Failed to load LLM providers:', error);
    // Initialize with default if failed
  }
};

// Toggle provider enabled status by unique fields (provider_type + base_url)
const toggleProviderEnabled = (providerToToggle: ProviderConfig) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToToggle.provider_type && p.base_url === providerToToggle.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].enabled = !articleAnalysisStore.providers[idx].enabled;
  }
};

// Save configuration
const saveSettings = async () => {
  try {
    isSubmitting.value = true;

    await SaveLLMProviders(articleAnalysisStore.providers);

    // Refresh configs from backend
    await Promise.all([fetchConfig(), loadLLMProviders()]);

    // Refresh models with the new API keys
    fetchAvailableModels();

    // Update the original state after saving
    storeOriginalState();

    toast({
      title: "Success",
      description: "Settings saved successfully!",
      variant: "default",
      duration: 1000
    });

  } catch (error) {
    console.error('Failed to save settings:', error);
    toast({
      title: "Error",
      description: `Failed to save settings: ${error}`,
      variant: "destructive"
    });
  } finally {
    isSubmitting.value = false;
  }
};

// Update provider model
const updateProviderModel = (providerToUpdate: ProviderConfig, modelName: string) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].model_name = modelName;
  }
};

// Update provider base URL
const updateProviderBaseUrl = (providerToUpdate: ProviderConfig, baseUrl: string) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].base_url = baseUrl;
  }
};

// Update provider temperature
const updateProviderTemperature = (providerToUpdate: ProviderConfig, temperature: number) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].temperature = Number(temperature);
  }
};

// Update provider max retries
const updateProviderMaxRetries = (providerToUpdate: ProviderConfig, maxRetries: number) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].max_retries = Number(maxRetries);
  }
};

// Update provider timeout minutes
const updateProviderTimeoutMinutes = (providerToUpdate: ProviderConfig, timeoutMinutes: number) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].timeout_minutes = Number(timeoutMinutes);
  }
};

// Update provider API key
const updateProviderApiKey = (providerToUpdate: ProviderConfig, apiKey: string) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].api_key = apiKey;
  }
};

// Update provider name
const updateProviderName = (providerToUpdate: ProviderConfig, name: string) => {
  const idx = articleAnalysisStore.providers.findIndex(
    (p) => p.provider_type === providerToUpdate.provider_type && p.base_url === providerToUpdate.base_url
  );
  if (idx !== -1) {
    articleAnalysisStore.providers[idx].name = name;
  }
};

// Filter models based on provider type
const getProviderModels = (providerType: string) => {
  return availableModels.value.filter(model => model.provider_type === providerType);
};

// Format model name for display
const formatModelDisplay = (model: ModelInfo) => {
  if (model.display_name) {
    return model.display_name;
  }

  // Return just the model name for clarity
  return model.name;
};

// Check if we should show model dropdown for this provider
const shouldShowModelDropdown = (providerType: string) => {
  return getProviderModels(providerType).length > 0;
};

// Check if we should show the API endpoint field for this provider
const shouldShowApiEndpoint = (providerType: string) => {
  return providerType === 'ollama' || providerType === 'llamacpp' || providerType === 'openai';
};
</script>

<template>
  <!-- Loading overlay - show when initial data is loading -->
    <div v-if="isLoading" class="text-center py-8">
      <LoaderIcon class="h-12 w-12 mb-4 mx-auto animate-spin" />
      <p>Loading LLM settings...</p>
  </div>

  <!-- Only show unsaved changes notification when not loading -->
  <div v-if="hasUnsavedChanges"
       class="fixed top-6 left-1/2 transform -translate-x-1/2 z-50 rounded-md p-3 bg-gray-800 border border-amber-500 flex justify-between items-center max-w-lg w-full transition-transform duration-300 ease-in-out"
       :class="hasUnsavedChanges ? 'translate-y-0' : '-translate-y-16'">
    <span class="font-medium text-gray-300">You have unsaved changes</span>
    <Button @click="saveSettings" :disabled="isSubmitting" variant="default">
      <span v-if="!isSubmitting">Save All Changes</span>
          <div v-else class="text-center">
            <LoaderIcon class="mx-auto animate-spin" />
          </div>
    </Button>
  </div>


  <!-- LLM Providers Section -->
  <section class="mb-8">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-lg font-medium">LLM Providers</h2>
    </div>

    <div class="space-y-4 bg-gray-900 p-4 rounded-lg">
      <!-- New Provider Selection - Restored with Add button -->

      <div class="p-3 rounded-md mb-4">
        <p class="text-sm text-gray-300 mb-2">Add a new LLM provider:</p>
        <div class="flex gap-2">
          <Select v-model="newProviderInput" @update:modelValue="(value) => value && fetchAvailableModels()" placeholder="Select a provider type" class="flex-grow">
            <SelectTrigger>
              <SelectValue placeholder="Select a provider type"/>
            </SelectTrigger>
            <SelectContent
              class="select-content-fix"
            >
              <SelectItem v-for="provider in availableProviders" :key="provider" :value="provider">
                <div
                  class="select-item-fix"
                >
                  {{ provider }}
                </div>
              </SelectItem>
            </SelectContent>
          </Select>
          <Button variant="default" @click="addProvider" :disabled="!newProviderInput">
            Add
          </Button>
        </div>
      </div>

      <div v-if="providers.length === 0" class="text-amber-500 text-sm py-2">
        No LLM providers configured. Please add at least one provider using the selector above.
      </div>

      <div v-else class="space-y-3">
        <div
            v-for="(provider, index) in sortedProviders"
            :key="`${provider.provider_type}-${index}`"
            class="p-3 border bg-gray-800 border-gray-700 rounded-md"
            :class="{
              'opacity-50': !provider.enabled
            }"
        >
          <div class="flex items-center justify-between mb-2">
            <div class="flex items-center gap-2">
              <Checkbox
                  :id="`provider-${index}`"
                  :modelValue="provider.enabled"
                  @update:modelValue="() => toggleProviderEnabled(provider)"
                  class="cursor-pointer"
              />
              <Label :for="`provider-${index}`" class="cursor-pointer">
                {{ provider.name || provider.provider_type }}/{{ provider.model_name || 'No model selected' }}
              </Label>
            </div>
            <div class="flex gap-2">
              <Button
                variant="destructive"
                size="sm"
                @click="removeProvider(provider)"
                class="h-8 px-2 text-xs"
            >
                Remove
            </Button>
            </div>
          </div>

          <div class="grid gap-2 mt-2">
            <Label :for="`provider-name-${index}`">Name</Label>
            <Input
                :id="`provider-name-${index}`"
                :modelValue="provider.name"
                @update:modelValue="(value) => updateProviderName(provider, value as string)"
                type="text"
                placeholder="e.g. My Local Ollama, GPT-4 Production..."
            />
          </div>

          <div class="grid gap-2 mt-2">
            <Label :for="`model-name-${index}`">Model</Label>

            <!-- Show loading indicator while fetching models for this provider -->
            <div v-if="loadingModelsState[providerKey(provider)]" class="flex items-center text-sm text-gray-400">
              <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
              Loading available models...
            </div>

            <!-- Show Select for providers with available models, Input for others -->
            <div v-else-if="shouldShowModelDropdown(provider.provider_type)">
              <Select
                  :modelValue="provider.model_name"
                  @update:modelValue="(value: AcceptableValue) => updateProviderModel(provider, value as string)"
              >
                <SelectTrigger class="w-full">
                  <SelectValue
                      :placeholder="provider.model_name || 'Select a model'"/>
                </SelectTrigger>
                <SelectContent
                  class="select-content-fix"
                >
                  <SelectItem value="auto">
                    <div class="flex flex-col select-item-fix">
                      <span class="font-medium">Auto</span>
                      <span class="text-xs text-muted-foreground">Use the first available model</span>
                    </div>
                  </SelectItem>
                  <SelectItem v-for="model in getProviderModels(provider.provider_type)" :key="model.id"
                              :value="model.name">
                    <div class="flex flex-col select-item-fix">
                      <span class="font-medium">{{ formatModelDisplay(model) }}</span>
                    </div>
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Input
                v-else-if="!loadingModelsState[providerKey(provider)]"
                :id="`model-name-${index}`"
                :modelValue="provider.model_name"
                @update:modelValue="(value) => updateProviderModel(provider, value as string)"
                type="text"
                placeholder="Enter model name (e.g., gpt-4o, claude-3-opus, deepseek-r1:14b, mistral-small-latest...)"
            />
            <p v-if="!loadingModelsState[providerKey(provider)] && !shouldShowModelDropdown(provider.provider_type)" class="text-xs text-muted-foreground mt-1">
              Use <code class="font-mono">auto</code> to automatically select the first available model at runtime.
            </p>

            <p v-if="!loadingModelsState[providerKey(provider)] && provider.provider_type === 'openai' && !getProviderModels('openai').length" class="text-xs text-amber-500 mt-1">
              No OpenAI models found. Please check your API key configuration.
            </p>
            <p v-if="!loadingModelsState[providerKey(provider)] && provider.provider_type === 'anthropic' && !getProviderModels('anthropic').length" class="text-xs text-amber-500 mt-1">
              No Anthropic models found. Please check your API key configuration.
            </p>
            <p v-if="!loadingModelsState[providerKey(provider)] && provider.provider_type === 'ollama' && !getProviderModels('ollama').length" class="text-xs text-amber-500 mt-1">
              No Ollama models found. Please ensure Ollama is running and accessible.
            </p>
            <p v-if="!loadingModelsState[providerKey(provider)] && provider.provider_type === 'mistral' && !getProviderModels('mistral').length" class="text-xs text-amber-500 mt-1">
              No Mistral models found. Defaulting to hardcoded model(s). Please check your API key configuration.
            </p>
            <p v-if="!loadingModelsState[providerKey(provider)] && provider.provider_type === 'llamacpp' && !getProviderModels('llamacpp').length" class="text-xs text-amber-500 mt-1">
              No llama.cpp models found. Ensure the server is running and the API endpoint is correct.
            </p>
          </div>

          <!-- Base URL input field - show for Ollama and llama.cpp -->
          <div v-if="shouldShowApiEndpoint(provider.provider_type)" class="grid gap-2 mt-2">
            <Label :for="`base-url-${index}`">API Endpoint</Label>
            <div class="flex gap-2 items-center">
              <Input
                  :id="`base-url-${index}`"
                  :modelValue="provider.base_url"
                  @update:modelValue="(value) => { updateProviderBaseUrl(provider, value as string); clearTestState(provider); }"
                  type="text"
                  :placeholder="provider.provider_type === 'llamacpp' ? 'http://localhost:8080' : provider.provider_type === 'openai' ? 'https://api.openai.com/v1' : 'http://localhost:11434'"
                  class="flex-1"
              />
              <Button
                variant="outline"
                size="sm"
                class="shrink-0 h-9 px-3 text-xs"
                :disabled="(!provider.base_url && provider.provider_type !== 'openai') || connectionTestState[providerKey(provider)]?.testing"
                @click="testConnection(provider)"
              >
                <LoaderIcon v-if="connectionTestState[providerKey(provider)]?.testing" class="h-3 w-3 animate-spin mr-1" />
                Verify
              </Button>
            </div>
            <!-- Test result badge -->
            <div v-if="connectionTestState[providerKey(provider)]?.result" class="flex items-center gap-1.5 text-xs mt-0.5">
              <CheckCircleIcon v-if="connectionTestState[providerKey(provider)]!.result!.success" class="h-3.5 w-3.5 text-green-500 shrink-0" />
              <XCircleIcon v-else class="h-3.5 w-3.5 text-red-500 shrink-0" />
              <span :class="connectionTestState[providerKey(provider)]!.result!.success ? 'text-green-400' : 'text-red-400'">
                {{ connectionTestState[providerKey(provider)]!.result!.message }}
              </span>
            </div>
            <p class="text-xs text-gray-400 mt-1">
              <span v-if="provider.provider_type === 'ollama'">Default Ollama endpoint is http://localhost:11434</span>
              <span v-else-if="provider.provider_type === 'llamacpp'">llama.cpp server endpoint (required). Default is http://localhost:8080</span>
              <span v-else-if="provider.provider_type === 'openai'">Optional — leave blank to use the default OpenAI endpoint. Useful for Azure OpenAI, proxies, or OpenAI-compatible services.</span>
            </p>
          </div>
          
          <!-- Per-provider API key -->
          <div class="grid gap-2 mt-2">
            <Label :for="`api-key-${index}`">API Key <span class="text-xs text-gray-400 font-normal">(overrides global key)</span></Label>
            <Input
                :id="`api-key-${index}`"
                :modelValue="provider.api_key"
                @update:modelValue="(value) => updateProviderApiKey(provider, value as string)"
                type="password"
                placeholder="Optional — leave blank to use the global key"
                autocomplete="off"
            />
          </div>

          <!-- Advanced Options Accordion -->
          <div class="mt-3">
            <Accordion type="single" collapsible class="w-full">
              <AccordionItem value="advanced-options">
                <AccordionTrigger class="text-sm font-medium py-2">
                  Advanced Options
                </AccordionTrigger>
                <AccordionContent>
                  <div class="space-y-3 pt-2">
                    <!-- Temperature Setting -->
                    <div class="grid gap-1">
                      <Label :for="`temperature-${index}`" class="text-xs">Temperature</Label>
                      <div class="flex items-center gap-2">
                        <Input
                          :id="`temperature-${index}`"
                          :modelValue="provider.temperature"
                          @update:modelValue="(value) => updateProviderTemperature(provider, value as number)"
                          type="number"
                          step="0.1"
                          min="0"
                          max="2"
                          class="w-full"
                        />
                      </div>
                      <p class="text-xs text-gray-400">
                        Controls randomness in responses (0-2, lower is more deterministic). Default: 0.3
                      </p>
                    </div>
                    
                    <!-- Max Retries Setting -->
                    <div class="grid gap-1">
                      <Label :for="`max-retries-${index}`" class="text-xs">Max Retries</Label>
                      <Input
                        :id="`max-retries-${index}`"
                        :modelValue="provider.max_retries"
                        @update:modelValue="(value) => updateProviderMaxRetries(provider, value as number)"
                        type="number"
                        min="0"
                        max="10"
                        class="w-full"
                      />
                      <p class="text-xs text-gray-400">
                        Number of retry attempts if requests fail. Default: 3
                      </p>
                    </div>
                    
                    <!-- Timeout Minutes Setting -->
                    <div class="grid gap-1">
                      <Label :for="`timeout-${index}`" class="text-xs">Timeout (minutes)</Label>
                      <Input
                        :id="`timeout-${index}`"
                        :modelValue="provider.timeout_minutes"
                        @update:modelValue="(value) => updateProviderTimeoutMinutes(provider, value as number)"
                        type="number"
                        min="1"
                        max="60"
                        class="w-full"
                      />
                      <p class="text-xs text-gray-400">
                        Maximum time to wait for response in minutes. Default: 5
                      </p>
                    </div>
                  </div>
                </AccordionContent>
              </AccordionItem>
            </Accordion>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>