<script setup lang="ts">
import { NCard } from 'naive-ui'

defineProps<{ label: string; value: string | number; unit?: string; desc?: string; accent?: string }>()
</script>

<template>
  <NCard size="small" :bordered="true" class="stat">
    <div class="label">{{ label }}</div>
    <div class="value" :style="{ color: accent || '#00e5ff' }">
      {{ value }}<span v-if="unit" class="unit">{{ unit }}</span>
    </div>
    <div v-if="desc" class="desc">{{ desc }}</div>
  </NCard>
</template>

<style scoped>
.stat {
  min-height: 92px;
}
.label {
  color: #8b9bb4;
  font-size: 12.5px;
  letter-spacing: 0.5px;
}
.value {
  font-family: 'JetBrains Mono', Menlo, monospace;
  font-size: 26px;
  font-weight: 600;
  line-height: 1.5;
}
.unit {
  font-size: 13px;
  margin-left: 4px;
  color: #8b9bb4;
}
.desc {
  color: #4a5568;
  font-size: 12px;
}
</style>
