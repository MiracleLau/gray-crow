<script setup lang="ts">
import { t } from '@/i18n'
import { computed, onMounted, ref } from 'vue'
import { keyApi, statsApi } from '@/api/endpoints'
import { NButton, NCard, NRadioButton, NRadioGroup, NSpace } from 'naive-ui'
import type { KeyUsage, TrendPoint } from '@/api/types'
import { areaSeries, axisStyle, baseOption } from '@/utils/echarts'
import { num } from '@/utils/format'
import BaseChart from '@/components/BaseChart.vue'
import StatCard from '@/components/StatCard.vue'

const trend = ref<TrendPoint[]>([])
const byRule = ref<{ rule_id: number; rule_name: string; total: number; pending: number }[]>([])
const byStatus = ref<{ status: string; total: number }[]>([])
const usage = ref<KeyUsage[]>([])
const range = ref('30d')

// 用函数而不是常量表：切换语言时 computed 会重新求值
const statusLabel = (status: string) => t(`status.${status}`)

const trendOption = computed(() => ({
  ...baseOption(),
  xAxis: { type: 'category', boundaryGap: false, data: trend.value.map((p) => p.date.slice(5)), ...axisStyle() },
  yAxis: { type: 'value', ...axisStyle() },
  series: [
    areaSeries(t('stats.chart_new'), trend.value.map((p) => p.new), 'rgb(0,229,255)'),
    areaSeries(t('status.resolved'), trend.value.map((p) => p.resolved), 'rgb(82,196,26)'),
    areaSeries(t('status.false_positive'), trend.value.map((p) => p.false_positive), 'rgb(250,173,20)'),
  ],
}))

const ruleOption = computed(() => ({
  ...baseOption(),
  grid: { left: 130, right: 30, top: 20, bottom: 24 },
  xAxis: { type: 'value', ...axisStyle() },
  yAxis: { type: 'category', data: byRule.value.map((r) => r.rule_name), ...axisStyle(), splitLine: { show: false } },
  series: [
    { name: t('stats.total_alerts'), type: 'bar', stack: 'a', barWidth: 14, data: byRule.value.map((r) => r.total), itemStyle: { color: '#00e5ff' } },
    { name: t('status.pending'), type: 'bar', stack: 'a', barWidth: 14, data: byRule.value.map((r) => r.pending), itemStyle: { color: '#faad14' } },
  ],
}))

const keyOption = computed(() => ({
  ...baseOption(),
  xAxis: { type: 'category', data: usage.value.map((u) => u.name), ...axisStyle() },
  yAxis: { type: 'value', ...axisStyle() },
  series: [
    { name: t('stats.used'), type: 'bar', barWidth: 16, data: usage.value.map((u) => u.total_used), itemStyle: { color: '#2f6bff' } },
    { name: t('stats.contributed'), type: 'bar', barWidth: 16, data: usage.value.map((u) => u.alerts), itemStyle: { color: '#00e5ff' } },
  ],
}))

async function load() {
  ;[trend.value, byRule.value, byStatus.value, usage.value] = await Promise.all([
    statsApi.trend(range.value),
    statsApi.byRule(),
    statsApi.byStatus(),
    keyApi.usage(range.value),
  ])
}

function exportCsv() {
  window.location.href = `/api/v1/stats/export?type=alerts&range=${range.value}`
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('stats.desc') }}</p>
      <NSpace>
        <NRadioGroup v-model:value="range" size="small" @update:value="load">
          <NRadioButton value="7d">{{ t('stats.range_7') }}</NRadioButton>
          <NRadioButton value="30d">{{ t('stats.range_30') }}</NRadioButton>
          <NRadioButton value="90d">{{ t('stats.range_90') }}</NRadioButton>
        </NRadioGroup>
        <NButton size="small" ghost type="primary" @click="exportCsv">{{ t('common.export_csv') }}</NButton>
      </NSpace>
    </div>

    <div class="grid-cards">
      <StatCard v-for="s in byStatus" :key="s.status" :label="statusLabel(s.status) || s.status" :value="num(s.total)" />
    </div>

    <NCard size="small" :title="t('stats.trend')" style="margin-top: 14px">
      <BaseChart :option="trendOption" :height="280" />
    </NCard>

    <div class="cols">
      <NCard size="small" :title="t('stats.by_rule')">
        <BaseChart :option="ruleOption" :height="240" />
      </NCard>
      <NCard size="small" :title="t('stats.key_usage')">
        <BaseChart :option="keyOption" :height="240" />
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 14px;
}
@media (max-width: 1100px) {
  .cols {
    grid-template-columns: 1fr;
  }
}
</style>
