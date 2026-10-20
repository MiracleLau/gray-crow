<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'
import { NTag } from 'naive-ui'
import type { AlertStatus, KeyStatus } from '@/api/types'

const props = defineProps<{ status: AlertStatus | KeyStatus | string }>()

// 只存 key：语言切换后 computed 会重新求值（写死 label 的话切语言不生效）
const map: Record<string, { key: string; type: 'default' | 'info' | 'success' | 'warning' | 'error' }> = {
  pending: { key: 'status.pending', type: 'warning' },
  resolved: { key: 'status.resolved', type: 'success' },
  false_positive: { key: 'status.false_positive', type: 'info' },
  ignored: { key: 'status.ignored', type: 'default' },
  allowlisted: { key: 'status.allowlisted', type: 'default' },
  active: { key: 'status.active', type: 'success' },
  low: { key: 'status.low', type: 'warning' },
  cooling: { key: 'status.cooling', type: 'warning' },
  invalid: { key: 'status.invalid', type: 'error' },
  disabled: { key: 'status.disabled', type: 'default' },
}

const item = computed(() => {
  const hit = map[props.status]
  return hit ? { label: t(hit.key), type: hit.type } : { label: props.status, type: 'default' as const }
})
</script>

<template>
  <NTag :type="item.type" size="small" :bordered="false">{{ item.label }}</NTag>
</template>
