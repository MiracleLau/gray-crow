<script setup lang="ts">
import { t } from '@/i18n'
import { h, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NCard, NDataTable, NInput, NModal, NPopconfirm, NSelect, NSpace, useMessage, useDialog } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { DataTableRowKey } from 'naive-ui'
import { alertApi, ruleApi } from '@/api/endpoints'
import type { Alert, AlertStatus, Rule } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import SnippetViewer from '@/components/SnippetViewer.vue'
import StatusTag from '@/components/StatusTag.vue'
import { dateTime, relTime } from '@/utils/format'

const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()

const loading = ref(false)
const rows = ref<Alert[]>([])
const rules = ref<Rule[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const checked = ref<DataTableRowKey[]>([])
const filters = reactive<{ status: AlertStatus | null; rule_id: number | null; repo: string }>({
  status: null,
  rule_id: null,
  repo: '',
})

const statusOptions: { label: string; value: AlertStatus }[] = [
  { label: t('status.pending'), value: 'pending' },
  { label: t('status.resolved'), value: 'resolved' },
  { label: t('status.false_positive'), value: 'false_positive' },
  { label: t('status.ignored'), value: 'ignored' },
  { label: t('status.allowlisted'), value: 'allowlisted' },
]

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: page.value, page_size: pageSize.value }
    Object.entries(filters).forEach(([k, v]) => {
      if (v !== null && v !== '') params[k] = v
    })
    const res = await alertApi.list(params)
    rows.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

async function bulk(status: AlertStatus, note = '') {
  if (!checked.value.length) {
    message.warning(t('alerts.pick_first'))
    return
  }
  const ids = checked.value.map(Number)
  await alertApi.bulkStatus(ids, status, note)
  message.success(t('alerts.updated', { n: ids.length }))
  checked.value = []
  await load()
}

// 历史不自动清理：勾选后由管理员确认删除，告警的时间线一并删除
function removeChecked() {
  if (!checked.value.length) {
    message.warning(t('alerts.pick_first'))
    return
  }
  const ids = checked.value.map(Number)
  dialog.warning({
    title: t('common.delete'),
    content: t('alerts.confirm_delete', { n: ids.length }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await alertApi.bulkDelete(ids)
      message.success(t('alerts.deleted', { n: ids.length }))
      checked.value = []
      await load()
    },
  })
}

const columns: DataTableColumns<Alert> = [
  { type: 'selection', width: 40 },
  { title: t('alerts.repo'), key: 'repo_full_name', width: 190, render: (row) => h('a', { class: 'mono link', href: repoURL(row), target: '_blank', rel: 'noopener noreferrer' }, row.repo_full_name) },
  { title: t('alerts.path'), key: 'path', ellipsis: { tooltip: true }, render: (row) => h('span', { class: 'mono link', onClick: () => openPreview(row) }, row.path) },
  { title: t('alerts.keyword'), key: 'matched_keyword', width: 140 },
  { title: t('alerts.rule'), key: 'rule_name', width: 170, ellipsis: { tooltip: true } },
  { title: t('common.status'), key: 'status', width: 92, render: (row) => h(StatusTag, { status: row.status }) },
  { title: t('alerts.first_seen'), key: 'first_seen_at', width: 130, render: (row) => h('span', { title: dateTime(row.first_seen_at) }, relTime(row.first_seen_at)) },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 150,
    render: (row) =>
      h('div', { class: 'row', style: 'gap:6px' }, [
        h(NButton, { size: 'tiny', ghost: true, type: 'primary', onClick: () => router.push(`/alerts/${row.id}`) }, { default: () => t('common.detail') }),
        h(
          NPopconfirm,
          { onPositiveClick: () => markOne(row.id, 'false_positive') },
          {
            trigger: () => h(NButton, { size: 'tiny', quaternary: true }, { default: () => t('status.false_positive') }),
            default: () => t('alerts.confirm_fp'),
          },
        ),
      ]),
  },
]

async function markOne(id: number, status: AlertStatus) {
  await alertApi.setStatus(id, status)
  message.success(t('alerts.updated'))
  await load()
}

const preview = ref<Alert | null>(null)
const previewShow = ref(false)

function openPreview(row: Alert) {
  preview.value = row
  previewShow.value = true
}

// 列表接口没给 repo_url 时按 GitHub 约定拼仓库地址
const repoURL = (row: Alert) => row.repo_url || `https://github.com/${row.repo_full_name}`

// 库里没存行号：用文本片段让浏览器滚到并高亮泄露内容，不支持的浏览器只是打开文件
function leakURL(row: Alert) {
  if (!row.html_url) return row.html_url
  const lines = (row.snippet || '').split('\n')
  const text = (lines.find((line) => line.includes(row.matched_keyword)) || lines.find((line) => line.trim()) || '').trim()
  if (!text) return row.html_url
  return row.html_url + (row.html_url.includes('#') ? '~:text=' : '#:~:text=') + encodeURIComponent(text)
}

function openLeak() {
  const url = preview.value ? leakURL(preview.value) : ''
  if (url) window.open(url, '_blank', 'noopener')
}

onMounted(async () => {
  await load()
  rules.value = (await ruleApi.list({ page: 1, page_size: 100 })).items
})
</script>

<template>
  <div class="page page-fill">
    <p class="page-desc">{{ t('alerts.desc') }}</p>

    <NCard size="small" style="margin-bottom: 14px; flex: none">
      <NSpace align="center" :wrap="true">
        <NSelect v-model:value="filters.status" :options="statusOptions" :placeholder="t('alerts.all_status')" clearable style="width: 130px" />
        <NSelect
          v-model:value="filters.rule_id"
          :options="rules.map((r) => ({ label: r.name, value: r.id }))"
          :placeholder="t('alerts.all_rules')"
          clearable
          style="width: 190px"
        />
        <NInput v-model:value="filters.repo" :placeholder="t('alerts.repo_placeholder')" clearable style="width: 200px" @keyup.enter="load" />
        <NButton type="primary" ghost @click="page = 1; load()">{{ t('common.search') }}</NButton>
        <NButton quaternary @click="Object.assign(filters, { status: null, rule_id: null, repo: '' }); load()">{{ t('common.reset') }}</NButton>
      </NSpace>
      <div class="bulk">
        <span class="mono">{{ t('alerts.selected', { n: checked.length }) }}</span>
        <NButton size="small" type="primary" :disabled="!checked.length" @click="bulk('resolved', t('alerts.bulk_resolved'))">{{ t('detail.mark_resolved') }}</NButton>
        <NButton size="small" type="info" :disabled="!checked.length" @click="bulk('false_positive', t('alerts.bulk_fp'))">{{ t('detail.mark_fp') }}</NButton>
        <NButton size="small" :disabled="!checked.length" @click="bulk('ignored', t('alerts.bulk_ignored'))">{{ t('status.ignored') }}</NButton>
        <NButton v-if="auth.isAdmin" size="small" type="error" ghost :disabled="!checked.length" @click="removeChecked">{{ t('common.delete') }}</NButton>
      </div>
    </NCard>

    <NCard size="small" class="table-fill">
      <NDataTable
        v-model:checked-row-keys="checked"
        remote
        flex-height
        style="flex: 1; min-height: 0"
        :loading="loading"
        :columns="columns"
        :data="rows"
        :row-key="(row: Alert) => row.id"
        :pagination="{ page, pageSize, itemCount: total, showSizePicker: true, pageSizes: [20, 50, 100], onUpdatePage: (p: number) => { page = p; load() }, onUpdatePageSize: (s: number) => { pageSize = s; page = 1; load() } }"
        :scroll-x="1120"
      />
    </NCard>

    <NModal v-model:show="previewShow" preset="card" :title="preview?.path" style="width: 720px">
      <div class="row" style="justify-content: space-between; gap: 12px; margin-bottom: 12px">
        <span class="mono" style="color: #8b9bb4; font-size: 12.5px">{{ preview?.rule_name }} · {{ preview?.repo_full_name }}</span>
        <NButton size="small" type="primary" ghost :disabled="!preview?.html_url" @click="openLeak">{{ t('detail.file') }}</NButton>
      </div>
      <SnippetViewer v-if="preview?.snippet" :snippet="preview.snippet" :keyword="preview.matched_keyword" />
      <p v-else class="page-desc" style="margin: 0">{{ t('alerts.no_snippet') }}</p>
    </NModal>
  </div>
</template>

<style scoped>
.bulk {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
  padding: 8px 12px;
  border: 1px solid rgba(0, 229, 255, 0.2);
  border-radius: 8px;
  background: rgba(0, 229, 255, 0.05);
}
</style>
