<script setup lang="ts">
import { t } from '@/i18n'
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NAlert, NButton, NCard, NCheckbox, NDataTable, NDescriptions, NDescriptionsItem, NDrawer, NDrawerContent, NDynamicTags, NForm, NFormItem, NInput, NInputNumber, NModal, NRadioButton, NRadioGroup, NSelect, NSpace, NSwitch, NTag, useMessage, useDialog } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { channelApi, ruleApi, userApi } from '@/api/endpoints'
import type { Channel, Rule, RuleDiagnosis, ScanRun, Severity, User } from '@/api/types'
import { dateTime, countdown, relTime } from '@/utils/format'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const diagnosis = ref<RuleDiagnosis | null>(null)
const diagnoseRule = ref<Rule | null>(null)
const diagnosing = ref(false)
const diagnosisOpen = ref(false)
const rows = ref<Rule[]>([])
const users = ref<User[]>([])
const channels = ref<Channel[]>([])
const drawer = ref(false)
const runsDrawer = ref(false)
const runs = ref<ScanRun[]>([])
const editing = ref<number | null>(null)
const form = reactive({
  name: '',
  description: '',
  keywords: [] as string[],
  query_template: '',
  schedule_type: 'interval' as 'interval' | 'cron',
  cron_expr: '',
  interval_sec: 3600 as number | null,
  run_now: true,
  severity: 'high' as Severity,
  enabled: true,
  dry_run: true,
  viewer_ids: [] as number[],
  channel_ids: [] as number[],
})

const cronPreview = ref<{ timezone: string; next: string[] } | null>(null)
let previewSeq = 0

/** cron 预览：后端算好接下来 3 次运行时间，前端不碰 cron 解析。 */
async function loadCronPreview() {
  const expr = form.cron_expr.trim()
  cronPreview.value = null
  if (form.schedule_type !== 'cron' || !expr) return
  const seq = ++previewSeq
  try {
    const data = await ruleApi.previewSchedule(expr)
    if (seq === previewSeq) cronPreview.value = data
  } catch {
    // 错误提示由请求层统一弹出
  }
}

// 与 internal/scanner/query.go 的 DefaultTemplate / MaxKeywords / BuildQueries 保持一致
const DEFAULT_TEMPLATE = '"{kw}" password -is:fork || "{kw}" secret -is:fork || "{kw}" token -is:fork || "{kw}" apikey -is:fork'
const MAX_KEYWORDS = 10

/** 模板用 || 分隔分支：每个关键词 × 每条分支一条查询。缺 {kw} 时拼到末尾、去重去空、封顶 10 个关键词。 */
function buildQueries(keywords: string[], template: string) {
  const branches = (template.trim() || DEFAULT_TEMPLATE)
    .split('||')
    .map((b) => b.trim())
    .filter(Boolean)
    .map((b) => (b.includes('{kw}') ? b : `${b} {kw}`))
  const seen = new Set<string>()
  const out: { keyword: string; text: string }[] = []
  for (const raw of keywords) {
    const kw = raw.trim()
    if (!kw || seen.has(kw)) continue
    seen.add(kw)
    for (const branch of branches) out.push({ keyword: kw, text: branch.replaceAll('{kw}', kw) })
    if (seen.size >= MAX_KEYWORDS) break
  }
  return out
}

const previewQueries = computed(() => buildQueries(form.keywords, form.query_template))

async function load() {
  loading.value = true
  try {
    rows.value = (await ruleApi.list({ page: 1, page_size: 100 })).items
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', description: '', keywords: [], query_template: '', schedule_type: 'interval', cron_expr: '', interval_sec: 3600, run_now: true, severity: 'high', enabled: true, dry_run: true, viewer_ids: [], channel_ids: [] })
  cronPreview.value = null
  drawer.value = true
}

function openEdit(row: Rule) {
  editing.value = row.id
  Object.assign(form, {
    name: row.name,
    description: row.description,
    keywords: [...row.keywords],
    query_template: row.query_template,
    schedule_type: row.schedule_type === 'cron' ? 'cron' : 'interval',
    cron_expr: row.cron_expr || '',
    interval_sec: row.interval_sec,
    run_now: true,
    severity: row.severity,
    enabled: row.enabled,
    dry_run: row.dry_run,
    viewer_ids: [...row.viewer_ids],
    channel_ids: [...row.channel_ids],
  })
  cronPreview.value = null
  drawer.value = true
  if (form.schedule_type === 'cron') void loadCronPreview()
}

async function save() {
  if (!form.name || !form.keywords.length) {
    message.warning(t('rules.need_input'))
    return
  }
  if (form.schedule_type === 'cron' && !form.cron_expr.trim()) {
    message.warning(t('rules.cron_required'))
    return
  }
  if (form.schedule_type === 'interval' && (form.interval_sec || 0) < 60) {
    message.warning(t('rules.interval_min'))
    return
  }
  const payload = { ...form, interval_sec: form.interval_sec || 3600 }
  if (editing.value) await ruleApi.update(editing.value, payload)
  else await ruleApi.create(payload)
  message.success(t('common.applied'))
  drawer.value = false
  await load()
}

async function toggle(row: Rule, field: 'enabled' | 'dry_run', value: boolean) {
  await ruleApi.update(row.id, { ...row, [field]: value })
  await load()
}

async function scan(row: Rule) {
  await ruleApi.scan(row.id)
  message.success(t('rules.queued'))
}

async function showRuns(row: Rule) {
  runs.value = (await ruleApi.runs(row.id, { page: 1, page_size: 20 })).items
  runsDrawer.value = true
}

async function showDiagnosis(row: Rule) {
  diagnosing.value = true
  try {
    diagnoseRule.value = row
    diagnosis.value = await ruleApi.diagnose(row.id)
    diagnosisOpen.value = true
  } finally {
    diagnosing.value = false
  }
}

const suggestionText = (item: RuleDiagnosis['suggestions'][number]) => {
  const pct = Math.round((item.value || 0) * 100)
  if (item.kind === 'drop_keyword') return t('rules.suggest.drop_keyword', { kw: item.keyword || '', pct })
  if (item.kind === 'keep_dry_run') return t('rules.suggest.keep_dry_run', { pct })
  if (item.kind === 'ready') return t('rules.suggest.ready')
  if (item.kind === 'ok') return t('rules.suggest.ok')
  return t('rules.suggest.no_data')
}

async function applyDropKeyword(keyword: string) {
  const row = diagnoseRule.value
  if (!row) return
  const keywords = row.keywords.filter((k) => k !== keyword)
  if (!keywords.length) {
    message.warning(t('rules.keep_one'))
    return
  }
  await ruleApi.update(row.id, { ...row, keywords })
  message.success(t('rules.keyword_removed', { kw: keyword }))
  await load()
  await showDiagnosis({ ...row, keywords })
}

function publishRule() {
  const row = diagnoseRule.value
  if (!row) return
  dialog.warning({
    title: t('rules.publish'),
    content: t('rules.publish_confirm'),
    positiveText: t('rules.publish'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await ruleApi.update(row.id, { ...row, dry_run: false })
      message.success(t('rules.published'))
      await load()
      await showDiagnosis({ ...row, dry_run: false })
    },
  })
}

const columns: DataTableColumns<Rule> = [
  { title: t('nav.rules'), key: 'name', width: 200 },
  { title: t('alerts.keyword'), key: 'keywords', render: (row) => h('div', { class: 'row', style: 'gap:4px;flex-wrap:wrap' }, row.keywords.map((k) => h('span', { class: 'mono kv' }, k))) },
  { title: t('rules.schedule'), key: 'schedule_type', width: 140, render: (row) => h('span', { class: 'mono', title: row.schedule_type === 'cron' ? row.cron_expr : `${row.interval_sec}s`, style: 'display:inline-block;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap' }, row.schedule_type === 'cron' ? `cron ${row.cron_expr}` : `${row.interval_sec / 60}m`) },
  { title: t('rules.hits'), key: 'alert_count', width: 72, render: (row) => h('span', { class: 'mono' }, row.alert_count) },
  { title: t('rules.next_scan'), key: 'next_scan_at', width: 110, render: (row) => h('span', { class: 'mono', title: dateTime(row.next_scan_at) }, countdown(row.next_scan_at)) },
  { title: t('common.enabled'), key: 'enabled', width: 70, render: (row) => h(NSwitch, { value: row.enabled, size: 'small', onUpdateValue: (v: boolean) => toggle(row, 'enabled', v) }) },
  { title: t('severity.dry_run'), key: 'dry_run', width: 130, render: (row) => h('div', { class: 'row', style: 'gap:6px' }, [h(NSwitch, { value: row.dry_run, size: 'small', onUpdateValue: (v: boolean) => toggle(row, 'dry_run', v) }), h('span', { style: 'font-size:11.5px;color:#8b9bb4' }, row.dry_run ? t('rules.dry_only') : t('rules.will_notify'))]) },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 260,
    render: (row) =>
      h('div', { class: 'row', style: 'gap:6px' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => t('common.edit') }),
        h(NButton, { size: 'tiny', quaternary: true, type: 'primary', onClick: () => scan(row) }, { default: () => t('rules.scan_now') }),
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => showRuns(row) }, { default: () => t('rules.runs') }),
        h(NButton, { size: 'tiny', quaternary: true, type: 'info', loading: diagnosing && diagnoseRule.value?.id === row.id, onClick: () => showDiagnosis(row) }, { default: () => t('rules.diagnose') }),
      ]),
  },
]

onMounted(async () => {
  await load()
  users.value = (await userApi.list({ page: 1, page_size: 100 })).items
  channels.value = await channelApi.list()
})
</script>

<template>
  <div class="page page-fill">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('rules.desc') }}</p>
      <NButton type="primary" ghost @click="openCreate">{{ t('rules.new') }}</NButton>
    </div>

    <NCard size="small" class="table-fill">
      <NDataTable flex-height style="flex: 1; min-height: 0" :loading="loading" :columns="columns" :data="rows" :row-key="(r: Rule) => r.id" :scroll-x="1300" />
    </NCard>

    <NDrawer v-model:show="drawer" :width="520">
      <NDrawerContent :title="editing ? t('rules.edit') : t('rules.new')" closable>
        <NForm label-placement="top">
          <NFormItem :label="t('rules.name')" required>
            <NInput v-model:value="form.name" :placeholder="t('rules.name_placeholder')" />
          </NFormItem>
          <NFormItem :label="t('common.description')">
            <NInput v-model:value="form.description" :placeholder="t('common.optional')" />
          </NFormItem>
          <NFormItem :label="t('rules.keywords')" required>
            <div style="width: 100%">
              <NDynamicTags v-model:value="form.keywords" :max="MAX_KEYWORDS" />
              <div class="hint">{{ t('rules.keywords_hint') }}</div>
            </div>
          </NFormItem>
          <NFormItem :label="t('rules.template')">
            <div style="width: 100%">
              <NInput v-model:value="form.query_template" :placeholder="DEFAULT_TEMPLATE" />
              <div class="hint">{{ t('rules.template_hint') }}</div>
            </div>
          </NFormItem>
          <NAlert type="default" :bordered="false" style="margin-bottom: 14px">
            <div v-if="!previewQueries.length" class="hint">{{ t('rules.preview_empty') }}</div>
            <template v-else>
              <div class="preview-title">{{ t('rules.preview_count', { n: previewQueries.length }) }}</div>
              <div class="preview-list">
                <div v-for="q in previewQueries" :key="q.keyword" class="preview-row">
                  <span class="preview-kw mono">{{ q.keyword }}</span>
                  <span class="preview-text mono">{{ q.text }}</span>
                </div>
              </div>
            </template>
          </NAlert>
          <NFormItem :label="t('rules.schedule')">
            <NRadioGroup v-model:value="form.schedule_type" @update:value="loadCronPreview">
              <NRadioButton value="interval">{{ t('rules.schedule_interval') }}</NRadioButton>
              <NRadioButton value="cron">{{ t('rules.schedule_cron') }}</NRadioButton>
            </NRadioGroup>
          </NFormItem>
          <NFormItem v-if="form.schedule_type === 'interval'" :label="t('rules.interval')">
            <NInputNumber v-model:value="form.interval_sec" :min="60" :step="60" style="width: 100%" />
          </NFormItem>
          <NFormItem v-else :label="t('rules.cron')">
            <div style="width: 100%">
              <NInput v-model:value="form.cron_expr" placeholder="0 9 * * 1-5" @blur="loadCronPreview" />
              <div class="hint">{{ t('rules.cron_hint') }}</div>
              <div v-if="cronPreview" class="hint cron-next">
                {{ t('rules.cron_next', { tz: cronPreview.timezone }) }}<br />
                <span class="mono">{{ cronPreview.next.join(', ') }}</span>
              </div>
            </div>
          </NFormItem>
          <NFormItem v-if="!editing" :label="t('rules.run_now')">
            <div>
              <NCheckbox v-model:checked="form.run_now">{{ t('rules.run_now_check') }}</NCheckbox>
              <div class="hint">{{ t('rules.run_now_hint') }}</div>
            </div>
          </NFormItem>
          <NFormItem :label="t('rules.dry_run')">
            <NSwitch v-model:value="form.dry_run" />
          </NFormItem>
          <NFormItem :label="t('common.enabled')">
            <NSwitch v-model:value="form.enabled" />
          </NFormItem>
          <NFormItem :label="t('rules.viewers')">
            <NSelect v-model:value="form.viewer_ids" multiple :options="users.map((u) => ({ label: `${u.nickname} (${u.username})`, value: u.id }))" :placeholder="t('rules.viewers_placeholder')" />
          </NFormItem>
          <NFormItem :label="t('nav.channels')">
            <NSelect v-model:value="form.channel_ids" multiple :options="channels.map((c) => ({ label: c.name, value: c.id }))" :placeholder="t('rules.channels_placeholder')" />
          </NFormItem>
        </NForm>
        <template #footer>
          <NButton type="primary" @click="save">{{ t('common.save') }}</NButton>
        </template>
      </NDrawerContent>
    </NDrawer>

    <NModal v-model:show="diagnosisOpen" preset="card" :title="t('rules.dry_title', { name: diagnoseRule?.name || '' })" style="width: 640px">
      <template v-if="diagnoseRule">
        <NDescriptions :column="3" size="small" bordered label-placement="top" style="margin-bottom: 14px">
          <NDescriptionsItem :label="t('rules.window')">{{ t('rules.window_days', { v: diagnosis?.window_days || 0 }) }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('rules.total')">{{ diagnosis?.total || 0 }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('rules.judged')">{{ diagnosis?.judged || 0 }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('rules.detected_context')">{{ diagnosis?.detected || 0 }} / {{ diagnosis?.context_only || 0 }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('rules.fp_count')">{{ diagnosis?.false_positive || 0 }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('rules.fp_rate')">{{ Math.round((diagnosis?.false_positive_rate || 0) * 100) }}%</NDescriptionsItem>
        </NDescriptions>

        <div class="diag-title">{{ t('rules.by_keyword') }}</div>
        <NDataTable
          size="small"
          :data="diagnosis?.by_keyword || []"
          :row-key="(r) => r.keyword"
          :columns="[
            { title: t('alerts.keyword'), key: 'keyword', render: (r) => h('span', { class: 'mono' }, r.keyword) },
            { title: t('rules.hits'), key: 'total', width: 70 },
            { title: t('rules.context_only'), key: 'context_only', width: 90 },
            { title: t('status.false_positive'), key: 'false_positive', width: 70 },
          ]"
        />
        <div class="diag-title">{{ t('rules.by_detector') }}</div>
        <div class="row" style="gap:6px;flex-wrap:wrap;margin-bottom:14px">
          <NTag v-for="d in diagnosis?.by_detector || []" :key="d.detector" size="small" :bordered="false">
            {{ d.detector || 'context-only' }} · {{ d.total }}
          </NTag>
        </div>

        <div class="diag-title">{{ t('rules.suggestions') }}</div>
        <NSpace vertical :size="8">
          <NAlert
            v-for="(s, i) in diagnosis?.suggestions || []"
            :key="i"
            :type="s.kind === 'ready' || s.kind === 'ok' ? 'success' : s.kind === 'keep_dry_run' ? 'warning' : 'default'"
            :bordered="false"
          >
            <div class="row" style="justify-content: space-between; gap: 10px">
              <span>{{ suggestionText(s) }}</span>
              <NButton v-if="s.kind === 'drop_keyword'" size="tiny" @click="applyDropKeyword(s.keyword!)">{{ t('rules.drop_keyword') }}</NButton>
              <NButton v-else-if="s.kind === 'ready'" size="tiny" type="primary" @click="publishRule">{{ t('rules.publish') }}</NButton>
            </div>
          </NAlert>
        </NSpace>
      </template>
    </NModal>

    <NDrawer v-model:show="runsDrawer" :width="720">
      <NDrawerContent :title="t('rules.runs')" closable>
        <NDataTable
          size="small"
          :data="runs"
          :row-key="(r: ScanRun) => r.id"
          :columns="[
            { title: t('rules.runs.time'), key: 'started_at', width: 160, render: (r: ScanRun) => relTime(r.started_at) },
            {
              title: t('common.status'),
              key: 'status',
              width: 76,
              render: (r: ScanRun) =>
                h(
                  'span',
                  { title: r.error || '', style: r.status === 'failed' ? 'color:#ff6b81' : r.status === 'partial' ? 'color:#f6c453' : 'color:#7a8ba5' },
                  r.status === 'failed' ? t('status.failed') : r.status === 'partial' ? t('status.partial') : t('status.success'),
                ),
            },
            { title: t('rules.runs.secret'), key: 'key_name', width: 110 },
            { title: t('rules.runs.result'), key: 'result_count', width: 80 },
            { title: t('rules.runs.new'), key: 'new_alert_count', width: 70 },
            { title: t('rules.runs.quota'), key: 'quota_remaining_after', width: 70 },
            { title: t('rules.runs.query'), key: 'query', ellipsis: { tooltip: true } },
          ]"
        />
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
.diag-title {
  margin: 4px 0 8px;
  color: #8b9bb4;
  font-size: 12px;
  letter-spacing: 1px;
}
.hint {
  margin-top: 4px;
  color: #8b9bb4;
  font-size: 11.5px;
  line-height: 1.5;
}
.cron-next {
  color: #5cf2ff;
}
.preview-title {
  margin-bottom: 6px;
  color: #8b9bb4;
  font-size: 12px;
  letter-spacing: 1px;
}
.preview-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 180px;
  overflow-y: auto;
}
.preview-row {
  display: flex;
  gap: 8px;
  align-items: baseline;
}
.preview-kw {
  flex: none;
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #5cf2ff;
  font-size: 11.5px;
}
.preview-text {
  color: #8b9bb4;
  font-size: 11.5px;
  word-break: break-all;
}
:deep(.kv) {
  padding: 1px 6px;
  border: 1px solid rgba(0, 229, 255, 0.2);
  border-radius: 4px;
  font-size: 11.5px;
  color: #c3d3e8;
}
</style>
