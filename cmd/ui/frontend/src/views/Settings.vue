<script setup lang="ts">
import { ref, onMounted, provide } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { ArrowLeft } from 'lucide-vue-next';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useToast } from "@/components/ui/toast";
// import General from '@/components/settings/General.vue';
import Feeds from '@/components/settings/Feeds.vue';
import LLMs from '@/components/settings/LLMProviders.vue';
import Analysis from '@/components/settings/Analysis.vue';

const router = useRouter();
const route = useRoute();
const { toast } = useToast();

// Global state for settings changes
const hasUnsavedChanges = ref(false);
const isSubmitting = ref(false);

// Provide these to child components
provide('hasUnsavedChanges', hasUnsavedChanges);
provide('setHasUnsavedChanges', (value: boolean) => {
  hasUnsavedChanges.value = value;
});

// Handle global save action
provide('saveSettings', async () => {
  // Child components will implement their own save logic
  // and call this function
  if (!saveCallback.value) {
    console.error('No save callback registered for the current tab');
    return;
  }
  
  try {
    isSubmitting.value = true;
    await saveCallback.value();
    
    toast({
      title: "Success",
      description: "Settings saved successfully!",
      variant: "default",
      duration: 1000
    });
    
    hasUnsavedChanges.value = false;
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
});

// Save callback reference
const saveCallback = ref<() => Promise<void>>(() => Promise.resolve());

// Register save callback from child components
provide('registerSaveCallback', (callback: () => Promise<void>) => {
  saveCallback.value = callback;
});

// Handle tab routing
const currentTab = ref(route.params.tab?.toString() || 'general');

const switchTab = (tab: string | number) => {
  const strTab = tab.toString()
  currentTab.value = strTab;
  router.push(`/settings/${strTab}`);
};

// Load initial tab based on route
onMounted(() => {
  if (route.params.tab) {
    currentTab.value = route.params.tab.toString();
  }
});

// Navigate back to previous page
const goBack = () => {
  router.push('/');
};
</script>

<template>
  <div class="flex flex-col h-full w-full">
    <!-- Global Unsaved Changes Notification -->
    <div v-if="hasUnsavedChanges"
         class="fixed top-6 left-1/2 transform -translate-x-1/2 z-50 rounded-md p-3 bg-gray-800 border border-amber-500 flex justify-between items-center max-w-lg w-full transition-transform duration-300 ease-in-out">
      <span class="font-medium text-gray-300">You have unsaved changes</span>
      <Button @click="saveCallback()" :disabled="isSubmitting" variant="default">
        <span v-if="isSubmitting">Saving...</span>
        <span v-else>Save All Changes</span>
      </Button>
    </div>
    
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b border-gray-800">
      <span class="flex items-center gap-2">
        <Button @click="goBack" class="px-2 gap-2 hover:bg-gray-800" variant="ghost" size="md">
          <ArrowLeft class="h-4 w-4" />
          Settings
        </Button>
      </span>
    </div>

    <!-- Tabs Navigation -->
    <div class="mx-auto mt-4 px-6 max-w-2xl">
      <Tabs default-value="feeds" :value="currentTab" @update:modelValue="switchTab" class="w-full">
        <TabsList class="grid grid-cols-3 w-full">
        <!-- <TabsList class="grid grid-cols-4 w-full"> -->
          <!-- <TabsTrigger class="cursor-pointer" value="general">General</TabsTrigger> -->
          <TabsTrigger class="cursor-pointer" value="feeds">Feeds</TabsTrigger>
          <TabsTrigger class="cursor-pointer" value="llm-providers">LLM</TabsTrigger>
          <TabsTrigger class="cursor-pointer" value="analysis">Analysis</TabsTrigger>
        </TabsList>
      </Tabs>
    </div>

    <!-- Settings Content -->
    <div class="flex-1 overflow-y-auto p-6">
      <div class="max-w-2xl mx-auto">
        <!-- General Settings -->
        <!-- <div  class="mt-0" :class="{ 'hidden': currentTab !== 'general' }">
          <General />
        </div> -->

        <!-- Feeds Settings -->
        <div  class="mt-0" :class="{ 'hidden': currentTab !== 'feeds' }">
          <Feeds />
        </div>

        <!-- LLM Providers Settings -->
        <div class="mt-0" :class="{ 'hidden': currentTab !== 'llm-providers' }">
          <LLMs />
        </div>

        <!-- Enrichment Settings -->
        <div class="mt-0" :class="{ 'hidden': currentTab !== 'analysis' }">
          <Analysis />
        </div>
      </div>
    </div>
  </div>
</template>