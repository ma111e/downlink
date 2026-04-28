<template>
  <svg
    :width="sizeValue"
    :height="sizeValue"
    :viewBox="viewboxValue"
  >
    <path :d="path" />
  </svg>
</template>

<script setup lang="ts">
// https://github.com/Pictogrammers/vue-icon/blob/master/lib/svg-icon.vue

import { computed } from 'vue';

interface Props {
  type?: 'mdi' | 'simple-icons' | 'default';
  path: string;
  size?: string | number;
  viewbox?: string;
  flip?: 'horizontal' | 'vertical' | 'both' | 'none';
  rotate?: number;
}

const props = withDefaults(defineProps<Props>(), {
  type: 'default',
  rotate: 0,
});

const types = new Map([
  ['mdi', { size: 24, viewbox: '0 0 24 24' }],
  ['simple-icons', { size: 24, viewbox: '0 0 24 24' }],
  ['default', { size: 0, viewbox: '0 0 0 0' }],
]);

const defaults = computed(() => {
  const t = types.get(props.type);
  if (!t) {
    throw new Error(`Unknown type ${props.type}`);
  } else {
    return t;
  }
});

const rotateValue = computed(() => {
  return isNaN(props.rotate) ? props.rotate : props.rotate + 'deg';
});

const scaleHorizontalValue = computed(() => {
  return props.flip && ['both', 'horizontal'].includes(props.flip) ? '-1' : '1';
});

const scaleVerticalValue = computed(() => {
  return props.flip && ['both', 'vertical'].includes(props.flip) ? '-1' : '1';
});

const sizeValue = computed(() => {
  return props.size || defaults.value.size;
});

const viewboxValue = computed(() => {
  return props.viewbox || defaults.value.viewbox;
});
</script>

<style scoped lang="css">
svg {
  transform: rotate(v-bind(rotateValue)) scale(v-bind(scaleHorizontalValue), v-bind(scaleVerticalValue));
}
path {
  fill: currentColor;
}
</style>
