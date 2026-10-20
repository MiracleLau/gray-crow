<script setup lang="ts">
import { t } from '@/i18n'
import { h, onMounted, reactive, ref } from 'vue'
import { NButton, NCard, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NRadioButton, NRadioGroup, NSelect, NSpace, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { DataTableRowKey } from 'naive-ui'
import { allowApi, ruleApi } from '@/api/endpoints'
import type { AllowEntry, Rule } from '@/api/types'
import { dateTime } from '@/utils/format'

const message = useMessage()
const loading = ref(false)
const rows = ref<AllowEntry[]>([])
const rules = ref<Rule[]>([])
const modal = ref(false)
const checked = ref<DataTableRowKey[]>([])
const form = reactive({ scope: 'global' as 'global' | 'rule', rule_id: null as number | null, repo_full_name: '', reason: '', expires_days: null as number | null })

async function load() {
  loading.value = true
  try {
    rows.value = (await allowApi.list({ page: 1, page_size: 100 })).items
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.repo_full_name.includes('/')) {
    message.warning(t('allow.repo_placeholder'))
    return
  }
  await allowApi.create({
    scope: form.scope,
    rule_id: form.scope === 'rule' ? form.rule_id || undefined : undefined,
    repo_full_name: form.repo_full_name,
    reason: form.reason,
    expires_at: form.expires_days ? new Date(Date.now() + form.expires_days * 86400000).toISOString() : undefined,
  })
  message.success(t('allow.added'))
  modal.value = false
  Object.assign(form, { scope: 'global', rule_id: null, repo_full_name: '', reason: '', expires_days: null })
  await load()
}

async function remove(id: number) {
  await allowApi.remove(id)
  message.success(t('allow.removed'))
  await load()
}

async function removeBulk() {
  const ids = checked.value.map(Number)
  await Promise.all(ids.map((id) => allowApi.remove(id)))
  message.success(t('allow.removed_count', { n: ids.length }))
  checked.value = []
  await load()
}

const columns: DataTableColumns<AllowEntry> = [
  { type: 'selection', width: 40 },
  { title: t('allow.scope'), key: 'scope', width: 130, render: (row) => (row.scope === 'global' ? h('span', t('allow.scope_global')) : h('span', `${t('allow.scope_prefix')}${row.rule_name || '-'}`)) },
  { title: t('alerts.repo'), key: 'repo_full_name', width: 220, render: (row) => h('span', { class: 'mono' }, row.repo_full_name) },
  { title: t('allow.reason'), key: 'reason', ellipsis: { tooltip: true } },
  { title: t('detail.allow_expires'), key: 'expires_at', width: 160, render: (row) => (row.expires_at ? dateTime(row.expires_at) : t('allow.forever')) },
  { title: t('allow.created_by'), key: 'created_by_name', width: 110 },
  { title: t('allow.created_at'), key: 'created_at', width: 160, render: (row) => dateTime(row.created_at) },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 90,
    render: (row) =>
      h(
        NPopconfirm,
        { onPositiveClick: () => remove(row.id) },
        { trigger: () => h(NButton, { size: 'tiny', quaternary: true, type: 'error' }, { default: () => t('allow.remove') }), default: () => t('allow.remove_confirm') },
      ),
  },
]

onMounted(async () => {
  await load()
  rules.value = (await ruleApi.list({ page: 1, page_size: 100 })).items
})
</script>

<template>
  <div class="page page-fill">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('allow.desc') }}</p>
      <NSpace>
        <NButton size="small" type="error" ghost :disabled="!checked.length" @click="removeBulk">{{ t('allow.bulk_remove', { n: checked.length }) }}</NButton>
        <NButton type="primary" ghost @click="modal = true">{{ t('allow.new') }}</NButton>
      </NSpace>
    </div>

    <NCard size="small" class="table-fill">
      <NDataTable v-model:checked-row-keys="checked" flex-height style="flex: 1; min-height: 0" :loading="loading" :columns="columns" :data="rows" :row-key="(r: AllowEntry) => r.id" />
    </NCard>

    <NModal v-model:show="modal" preset="card" :title="t('allow.new')" style="width: 480px">
      <NForm label-placement="top">
        <NFormItem :label="t('allow.scope')">
          <NRadioGroup v-model:value="form.scope">
            <NRadioButton value="global">{{ t('allow.scope_global') }}</NRadioButton>
            <NRadioButton value="rule">{{ t('allow.scope_rule') }}</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem v-if="form.scope === 'rule'" :label="t('allow.rule')">
          <NSelect v-model:value="form.rule_id" :options="rules.map((r) => ({ label: r.name, value: r.id }))" />
        </NFormItem>
        <NFormItem :label="t('allow.repo')" required>
          <NInput v-model:value="form.repo_full_name" placeholder="acme/docs" />
        </NFormItem>
        <NFormItem :label="t('allow.reason')">
          <NInput v-model:value="form.reason" :placeholder="t('allow.reason_placeholder')" />
        </NFormItem>
        <NFormItem :label="t('allow.days')">
          <NInputNumber v-model:value="form.expires_days" :min="1" style="width: 100%" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NButton type="primary" @click="save">{{ t('common.save') }}</NButton>
      </template>
    </NModal>
  </div>
</template>
