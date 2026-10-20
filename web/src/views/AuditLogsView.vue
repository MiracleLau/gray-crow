<script setup lang="ts">
import { t } from '@/i18n'
import { onMounted, reactive, ref } from 'vue'
import { auditApi } from '@/api/endpoints'
import { NButton, NCard, NDataTable, NDatePicker, NSelect, NSpace } from 'naive-ui'
import type { AuditLog } from '@/api/types'
import { dateTime } from '@/utils/format'

const loading = ref(false)
const rows = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const filters = reactive<{ action: string; from: string | null; to: string | null }>({ action: '', from: null, to: null })

const actions = ['login', 'logout', 'alert.status', 'alert.delete', 'notify.delete', 'rule.create', 'rule.update', 'apikey.check', 'allowlist.create', 'user.update']

async function load() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: page.value, page_size: pageSize.value }
    if (filters.action) params.action = filters.action
    if (filters.from) params.from = filters.from
    if (filters.to) params.to = filters.to
    const res = await auditApi.list(params)
    rows.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page page-fill">
    <p class="page-desc">{{ t('audit.desc') }}</p>

    <NCard size="small" style="margin-bottom: 14px; flex: none">
      <NSpace align="center">
        <NSelect
          v-model:value="filters.action"
          :options="actions.map((a) => ({ label: a, value: a }))"
          :placeholder="t('audit.all_actions')"
          clearable
          style="width: 180px"
        />
        <NDatePicker v-model:formatted-value="filters.from" type="date" value-format="yyyy-MM-dd" :placeholder="t('audit.from')" clearable />
        <NDatePicker v-model:formatted-value="filters.to" type="date" value-format="yyyy-MM-dd" :placeholder="t('audit.to')" clearable />
        <NButton type="primary" ghost @click="page = 1; load()">{{ t('common.search') }}</NButton>
      </NSpace>
    </NCard>

    <NCard size="small" class="table-fill">
      <NDataTable
        remote
        flex-height
        style="flex: 1; min-height: 0"
        :loading="loading"
        :data="rows"
        :row-key="(r: AuditLog) => r.id"
        :columns="[
          { title: t('audit.time'), key: 'created_at', width: 180, render: (r: AuditLog) => dateTime(r.created_at) },
          { title: t('audit.actor'), key: 'actor_name', width: 130 },
          { title: t('audit.action'), key: 'action', width: 160, render: (r: AuditLog) => r.action },
          { title: t('audit.target'), key: 'target', width: 220 },
          { title: t('common.detail'), key: 'detail', ellipsis: { tooltip: true } },
          { title: t('audit.ip'), key: 'ip', width: 130 },
        ]"
        :pagination="{ page, pageSize, itemCount: total, onUpdatePage: (p: number) => { page = p; load() } }"
      />
    </NCard>
  </div>
</template>
