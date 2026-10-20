<script setup lang="ts">
import { t } from '@/i18n'
import { NButton } from 'naive-ui'

defineProps<{ text?: string; action?: string }>()
defineEmits<{ action: [] }>()
</script>

<template>
  <div class="empty">
    <div class="icon">◤◥</div>
    <div class="text">{{ text || t('common.no_data') }}</div>
    <NButton v-if="action" size="small" type="primary" ghost @click="$emit('action')">{{ action }}</NButton>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 44px 0;
  color: #8b9bb4;
}
.icon {
  font-size: 26px;
  color: rgba(0, 229, 255, 0.35);
  letter-spacing: -4px;
}
.text {
  font-size: 13px;
}
</style>
