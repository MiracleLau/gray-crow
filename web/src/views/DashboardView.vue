<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NList, NListItem, NTag } from 'naive-ui'
import { statsApi, keyApi, ruleApi } from '@/api/endpoints'
import type { AlertStatus, ApiKey, Overview, QuotaPool, Rule, TrendPoint } from '@/api/types'
import { areaSeries, axisStyle, baseOption } from '@/utils/echarts'
import { t } from '@/i18n'
import { countdown, dateTime, num } from '@/utils/format'
import BaseChart from '@/components/BaseChart.vue'
import EmptyHint from '@/components/EmptyHint.vue'
import QuotaRing from '@/components/QuotaRing.vue'
import StatCard from '@/components/StatCard.vue'
import StatusTag from '@/components/StatusTag.vue'

const router = useRouter()
const overview = ref<Overview | null>(null)
const trend = ref<TrendPoint[]>([])
const quota = ref<QuotaPool | null>(null)
const byStatus = ref<{ status: AlertStatus; total: number }[]>([])
const rules = ref<Rule[]>([])
let timer: number | undefined

const statusColors: Record<string, string> = {
  pending: '#faad14',
  resolved: '#52c41a',
  false_positive: '#40a9ff',
  ignored: '#8b9bb4',
  allowlisted: '#00e5ff',
}
const statusLabel = (status: AlertStatus) => t(`status.${status}`)
const statusColor = (status: AlertStatus) => statusColors[status] || '#8b9bb4'

const accounts = computed(() => {
  const map = new Map<string, ApiKey[]>()
  quota.value?.keys.filter((k) => k.enabled).forEach((k) => map.set(k.account_login, [...(map.get(k.account_login) || []), k]))
  return [...map.entries()].map(([login, keys]) => ({
    login,
    keys,
    remaining: keys.reduce((s, k) => s + k.quota_remaining, 0),
    limit: keys.reduce((s, k) => s + k.quota_limit, 0),
  }))
})

const trendOption = computed(() => ({
  ...baseOption(),
  xAxis: { type: 'category', boundaryGap: false, data: trend.value.map((p) => p.date.slice(5)), ...axisStyle() },
  yAxis: { type: 'value', ...axisStyle() },
  series: [
    areaSeries(t('dashboard.chart_new'), trend.value.map((p) => p.new), 'rgb(0,229,255)'),
    areaSeries(t('status.resolved'), trend.value.map((p) => p.resolved), 'rgb(82,196,26)'),
    areaSeries(t('status.false_positive'), trend.value.map((p) => p.false_positive), 'rgb(250,173,20)'),
  ],
}))

const statusOption = computed(() => ({
  ...baseOption(),
  tooltip: { backgroundColor: 'rgba(17,23,38,.94)', borderColor: 'rgba(0,229,255,.3)', textStyle: { color: '#e6f1ff', fontSize: 12 }, trigger: 'item' },
  legend: { bottom: 0, textStyle: { color: '#8b9bb4' } },
  series: [
    {
      type: 'pie',
      radius: ['52%', '74%'],
      center: ['50%', '44%'],
      label: { color: '#8b9bb4', fontSize: 11 },
      itemStyle: { borderColor: '#0a0e17', borderWidth: 2 },
      data: byStatus.value.map((p) => ({ name: statusLabel(p.status), value: p.total, itemStyle: { color: statusColor(p.status) } })),
    },
  ],
}))

async function load() {
  ;[overview.value, trend.value, quota.value, byStatus.value] = await Promise.all([
    statsApi.overview(),
    statsApi.trend(),
    keyApi.quota(),
    statsApi.byStatus(),
  ])
  rules.value = (await ruleApi.list({ page: 1, page_size: 5 })).items
}

onMounted(async () => {
  await load()
  timer = window.setInterval(async () => {
    quota.value = await keyApi.quota()
  }, 5000)
})

onUnmounted(() => timer && clearInterval(timer))
</script>

<template>
  <div class="page">
    <p class="page-desc">{{ t('dashboard.desc') }}</p>

    <div class="grid-cards">
      <StatCard :label="t('dashboard.today_new')" :value="num(overview?.today_new)" />
      <StatCard :label="t('dashboard.pending')" :value="num(overview?.pending)" accent="#faad14" :desc="t('dashboard.pending_tip')" />
      <StatCard :label="t('dashboard.fp_rate')" :value="((overview?.false_positive_rate || 0) * 100).toFixed(0)" unit="%" accent="#40a9ff" />
      <StatCard :label="t('dashboard.resolved_rate')" :value="((overview?.resolved_rate || 0) * 100).toFixed(0)" unit="%" accent="#52c41a" />
      <StatCard :label="t('dashboard.avg_handle')" :value="num(overview?.avg_handle_minutes)" :unit="t('unit.minute')" accent="#8b9bb4" />
    </div>

    <div class="cols">
      <NCard size="small" :title="t('dashboard.trend')">
        <BaseChart :option="trendOption" :height="252" />
      </NCard>
      <NCard size="small" :title="t('dashboard.by_status')">
        <BaseChart :option="statusOption" :height="252" />
      </NCard>
    </div>

    <div class="cols">
      <NCard size="small">
        <template #header>
          <div class="row" style="justify-content: space-between; width: 100%">
            <span>{{ t('dashboard.pool') }}</span>
            <span class="mono" style="color: #8b9bb4">
              {{ num(overview?.pool_remaining) }}/{{ num(overview?.pool_limit) }} ·
              {{ t('dashboard.account_suffix', { n: overview?.pool_accounts || 0 }) }}{{ dateTime(quota?.updated_at) }}
            </span>
          </div>
        </template>
        <NAlert type="info" :bordered="false" style="margin-bottom: 12px">
          {{ t('dashboard.pool_note', { n: accounts.length, r: accounts.length * 10 }) }}
        </NAlert>
        <div v-for="a in accounts" :key="a.login" class="acct">
          <QuotaRing :remaining="a.remaining" :limit="a.limit" :size="66" />
          <div class="acct-info">
            <div class="acct-name mono">{{ a.login }}</div>
            <div class="acct-keys">
              <StatusTag v-for="k in a.keys" :key="k.id" :status="k.status" />
              <span class="mono" style="color: #4a5568">{{ t('dashboard.keys_count', { n: a.keys.length, v: countdown(a.keys[0]?.quota_reset_at) }) }}</span>
            </div>
          </div>
        </div>
        <NButton size="small" ghost type="primary" @click="router.push('/api-keys')">{{ t('dashboard.manage_pool') }}</NButton>
      </NCard>

      <NCard size="small" :title="t('dashboard.rule_health')">
        <NList v-if="rules.length" :show-divider="false">
          <NListItem v-for="r in rules" :key="r.id">
            <div class="row" style="justify-content: space-between">
              <div>
                <div>{{ r.name }}</div>
                <div class="mono" style="color: #4a5568; font-size: 11.5px">
                  {{ t('dashboard.last_scan', { v: r.last_scan_at ? dateTime(r.last_scan_at) : t('dashboard.not_scan'), i: `${r.interval_sec / 60}${t('unit.minute')}` }) }}
                </div>
              </div>
              <div class="row" style="gap: 8px">
                <NTag v-if="r.dry_run" size="small" type="warning" :bordered="false">{{ t('dashboard.dry_running') }}</NTag>
                <NTag v-if="!r.enabled" size="small" :bordered="false">{{ t('common.disabled') }}</NTag>
                <span class="mono">{{ t('dashboard.hits', { v: r.alert_count }) }}</span>
              </div>
            </div>
          </NListItem>
        </NList>
        <EmptyHint v-else :text="t('dashboard.no_rules')" :action="t('dashboard.new_rule')" @action="router.push('/rules')" />
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.cols {
  display: grid;
  grid-template-columns: 1.6fr 1fr;
  gap: 14px;
  margin-top: 14px;
}
@media (max-width: 1100px) {
  .cols {
    grid-template-columns: 1fr;
  }
}
.acct {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px 0;
  border-bottom: 1px dashed rgba(0, 229, 255, 0.1);
}
.acct-name {
  color: #00e5ff;
  margin-bottom: 6px;
}
.acct-keys {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
</style>
