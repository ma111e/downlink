<script setup lang="ts">
import { ref } from 'vue';
import { SearchIcon, ArrowRightIcon } from 'lucide-vue-next';

const props = defineProps<{
  modelValue: string;
  placeholder: string;
  debounce?: number; // Optional debounce time in milliseconds
}>();

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void;
  (e: 'search'): void;
}>();

// Default debounce time is 300ms if not provided
const debounceTime = props.debounce ?? 300;
const debounceTimer = ref<number | null>(null);

const updateValue = (event: Event) => {
  const target = event.target as HTMLInputElement;
  emit('update:modelValue', target.value);
  
  // Clear any existing timer
  if (debounceTimer.value) {
    clearTimeout(debounceTimer.value);
  }
  
  // Set a new timer for the debounced search
  debounceTimer.value = setTimeout(() => {
    emit('search');
  }, debounceTime) as unknown as number;
};

const handleKeyup = (event: KeyboardEvent) => {
  if (event.key === 'Enter') {
    // Clear any pending debounce timer
    if (debounceTimer.value) {
      clearTimeout(debounceTimer.value);
      debounceTimer.value = null;
    }
    emit('search');
  }
};
</script>

<template>
  <div class="px-3 py-4 border-b border-gray-800">
    <div class="flex items-center px-2 py-1 bg-gray-900 rounded-md">
      <SearchIcon class="w-4 h-4 text-gray-400" />
      <input
        :value="props.modelValue"
        @input="updateValue"
        @keyup="handleKeyup"
        type="text"
        :placeholder="props.placeholder"
        class="bg-transparent border-none outline-none px-2 p-1 w-full text-sm"
      />
      <button @click="emit('search')" class="text-gray-400">
        <ArrowRightIcon class="w-4 h-4" />
      </button>
    </div>
  </div>
</template>