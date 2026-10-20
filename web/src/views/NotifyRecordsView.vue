<script setup lang="ts">
import { t } from '@/i18n'
import { h, onMounted, reactive, ref } from 'vue'
import { notifyApi, ruleApi } from '@/api/endpoints'
import { NButton, NCard, NDataTable, NSelect, NSpace, NTag, useDialog, useMessage } from 'naive-ui'
import type { DataTableRowKey } from 'naive-ui'
import type { NotifyRecord, Rule } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { dateTime, num } from '@/utils/format'

const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()
const loading = ref(false)
const rows = ref<NotifyRecord[]>([])
const rules = ref<Rule[]>([])
const checked = ref<DataTableRowKey[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const filters = reactive<{ rule_id: number | null; status: string | null }>({ rule_id: null, status: null })

const statusOptions = [
  { label: t('notify.status_success'), value: 'success' },
  { label: t('notify.status_failed'), value: 'failed' },
]

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: page.value, page_size: pageSize.value }
    if (filters.rule_id) params.rule_id = filters.rule_id
    if (filters.status) params.status = filters.status
    const res = await notifyApi.list(params)
    rows.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

async function removeChecked() {
  if (!checked.value.length) {
    message.warning(t('alerts.pick_first'))
    return
  }
  const ids = checked.value.map(Number)
  dialog.warning({
    title: t('common.delete'),
    content: t('notify.confirm_delete', { n: ids.length }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await notifyApi.bulkDelete(ids)
      message.success(t('notify.deleted', { n: ids.length }))
      checked.value = []
      await load()
    },
  })
}

onMounted(async () => {
  await load()
  const res = await ruleApi.list({ page: 1, page_size: 200 })
  rules.value = res.items
})
</script>

<template>
  <div class="page page-fill">
    <p class="page-desc">{{ t('notify.desc') }}</p>

    <NCard size="small" style="margin-bottom: 14px; flex: none">
      <NSpace align="center">
        <NSelect
          v-model:value="filters.rule_id"
          :options="rules.map((r) => ({ label: r.name, value: r.id }))"
          :placeholder="t('notify.all_rules')"
          clearable
          style="width: 200px"
        />
        <NSelect v-model:value="filters.status" :options="statusOptions" :placeholder="t('notify.all_status')" clearable style="width: 140px" />
        <NButton type="primary" ghost @click="page = 1; load()">{{ t('common.search') }}</NButton>
      </NSpace>
      <div v-if="auth.isAdmin" class="bulk">
        <span class="mono">{{ t('alerts.selected', { n: checked.length }) }}</span>
        <NButton size="small" type="error" ghost :disabled="!checked.length" @click="removeChecked">{{ t('common.delete') }}</NButton>
      </div>
    </NCard>

    <NCard size="small" class="table-fill">
      <NDataTable
        v-model:checked-row-keys="checked"
        remote
        flex-height
        style="flex: 1; min-height: 0"
        :loading="loading"
        :data="rows"
        :row-key="(r: NotifyRecord) => r.id"
        :columns="[
          ...(auth.isAdmin ? [{ type: 'selection' as const, width: 40 }] : []),
          { title: t('notify.time'), key: 'sent_at', width: 180, render: (r: NotifyRecord) => dateTime(r.sent_at) },
          { title: t('notify.rule'), key: 'rule_name', width: 160, ellipsis: { tooltip: true } },
          { title: t('notify.channel'), key: 'channel_name', width: 140 },
          { title: t('notify.count'), key: 'alert_count', width: 100, render: (r: NotifyRecord) => h('span', { class: 'mono' }, num(r.alert_count)) },
          {
            title: t('common.status'),
            key: 'status',
            width: 120,
            render: (r: NotifyRecord) =>
              h(NTag, { size: 'small', type: r.status === 'success' ? 'success' : 'error', bordered: false },
                { default: () => (r.status === 'success' ? t('notify.status_success') : t('notify.status_failed')) }),
          },
          { title: t('notify.attempt'), key: 'attempt', width: 90, render: (r: NotifyRecord) => h('span', { class: 'mono' }, `#${r.attempt}`) },
          { title: t('notify.http'), key: 'http_status', width: 90, render: (r: NotifyRecord) => h('span', { class: 'mono' }, r.http_status ? String(r.http_status) : '-') },
          { title: t('notify.content'), key: 'content', ellipsis: { tooltip: { width: 520 } } },
          { title: t('notify.error'), key: 'error', width: 200, ellipsis: { tooltip: true } },
        ]"
        :pagination="{ page, pageSize, itemCount: total, onUpdatePage: (p: number) => { page = p; load() } }"
      />
    </NCard>
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
