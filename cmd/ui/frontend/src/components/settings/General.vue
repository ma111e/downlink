<script setup lang="ts">
import { ref } from 'vue';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
// import { useConfig } from '@/composables/useConfig';
// import { SaveConfig } from "../../../wailsjs/go/downlinkclient/DownlinkClient";

// const { models, _, fetchConfig } = useConfig();

const darkMode = ref(true);
const refreshInterval = ref(30);
const maxArticles = ref(100);
const isSubmitting = ref(false);
const errorMessage = ref('');

// Load models when component mounts
// fetchConfig().then(() => {
//   if (models.value) {
//     darkMode.value = models.value.darkMode || true;
//     refreshInterval.value = models.value.refreshInterval || 30;
//     maxArticles.value = models.value.maxArticles || 100;
//   }
// });

// Save general settings
const saveGeneralSettings = async () => {
  // try {
  //   isSubmitting.value = true;
  //   errorMessage.value = '';
  //
  //   if (!models.value) {
  //     errorMessage.value = 'Configuration not loaded';
  //     return;
  //   }
  //
  //   // Update models values
  //   models.value.darkMode = darkMode.value;
  //   models.value.refreshInterval = refreshInterval.value;
  //   models.value.maxArticles = maxArticles.value;
  //
  //   // Save the updated models
  //   await SaveConfig(models.value);
  //
  //   // Refresh models to reflect changes
  //   await fetchConfig();
  //
  // } catch (error) {
  //   console.error('Failed to save settings:', error);
  //   errorMessage.value = `Failed to save settings: ${error}`;
  // } finally {
  //   isSubmitting.value = false;
  // }
};
</script>

<template>
  <section class="mb-8">
    <h2 class="text-lg font-medium mb-4">General Settings</h2>
    <div class="space-y-4 bg-gray-900 p-4 rounded-lg">
      <div class="flex items-center space-x-2">
        <Switch id="dark-mode" v-model="darkMode" />
        <Label for="dark-mode">Dark Mode</Label>
      </div>

      <div class="grid gap-2">
        <Label for="refresh-interval">Auto Refresh Interval (minutes)</Label>
        <Input
          id="refresh-interval"
          v-model="refreshInterval"
          type="number"
          min="1"
          max="120"
        />
      </div>

      <div class="grid gap-2">
        <Label for="max-articles">Maximum Articles per Feed</Label>
        <Input
          id="max-articles"
          v-model="maxArticles"
          type="number"
          min="10"
          max="500"
        />
      </div>

      <div v-if="errorMessage" class="text-red-500 text-sm">
        {{ errorMessage }}
      </div>

      <div class="flex justify-end gap-2">
        <Button @click="saveGeneralSettings" :disabled="isSubmitting">
          <span v-if="isSubmitting">Saving...</span>
          <span v-else>Save Settings</span>
        </Button>
      </div>
    </div>
  </section>
</template>