<script setup lang="ts">
defineProps<{
  message: string;
}>();
</script>

<template>
  <div class="flex justify-center items-center p-4 text-gray-400">
    {{ message }}
  </div>
</template>