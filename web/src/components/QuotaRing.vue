<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'
import { NProgress } from 'naive-ui'

const props = defineProps<{ remaining: number; limit: number; size?: number }>()
const percent = computed(() => (props.limit ? Math.round((props.remaining / props.limit) * 100) : 0))
const color = computed(() => (percent.value > 50 ? '#00e5ff' : percent.value > 20 ? '#faad14' : '#ff4d4f'))
// naive-ui 圆形 Progress 的宽度在样式里写死 120px，theme 覆盖只改图标字号，只能用容器宽度控制
const box = computed(() => ({ width: `${props.size || 76}px` }))
</script>

<template>
  <NProgress
    type="circle"
    :percentage="percent"
    :stroke-width="10"
    :color="color"
    :rail-color="'rgba(0,229,255,.12)'"
    :show-indicator="true"
    :style="box"
  >
    <template #default>
      <div class="ring">
        <div class="num">{{ remaining }}<span>/{{ limit }}</span></div>
        <div class="cap">{{ t('stats.remaining') }}</div>
      </div>
    </template>
  </NProgress>
</template>

<style scoped>
/* 环内文字绝对居中，可用宽度只有半圈，不换行才不会被挤成两行 */
.ring {
  white-space: nowrap;
  text-align: center;
}
.num {
  font-family: 'JetBrains Mono', Menlo, monospace;
  font-size: 16px;
  font-weight: 600;
}
.num span {
  font-size: 11px;
  color: #8b9bb4;
}
.cap {
  font-size: 10.5px;
  color: #8b9bb4;
}
</style>
