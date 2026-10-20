<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ snippet: string; keyword?: string }>()

const html = computed(() => {
  const esc = props.snippet.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c] as string)
  if (!props.keyword) return esc
  const safe = props.keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return esc.replace(new RegExp(safe, 'gi'), (m) => `<mark>${m}</mark>`)
})
</script>

<template>
  <pre class="snippet mono" v-html="html" />
</template>

<style scoped>
.snippet {
  margin: 0;
  padding: 12px 14px;
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid rgba(0, 229, 255, 0.15);
  border-left: 2px solid #00e5ff;
  border-radius: 8px;
  color: #c3d3e8;
  white-space: pre-wrap;
  word-break: break-all;
  line-height: 1.65;
  max-height: 320px;
  overflow: auto;
}
.snippet :deep(mark) {
  background: rgba(255, 77, 79, 0.28);
  color: #ffb3b5;
  padding: 0 2px;
  border-radius: 2px;
}
</style>
