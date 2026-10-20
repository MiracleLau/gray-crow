<script setup lang="ts">
import { t } from '@/i18n'
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, NModal, NSpace, useDialog, useMessage } from 'naive-ui'
import { keyApi } from '@/api/endpoints'
import type { ApiKey, QuotaPool } from '@/api/types'
import { countdown, dateTime, num, relTime } from '@/utils/format'
import QuotaRing from '@/components/QuotaRing.vue'
import StatCard from '@/components/StatCard.vue'
import StatusTag from '@/components/StatusTag.vue'

const message = useMessage()
const dialog = useDialog()

const pool = ref<QuotaPool | null>(null)
const loading = ref(false)
const modal = ref(false)
const editing = ref<ApiKey | null>(null)
const form = reactive({ name: '', token: '' })
const now = ref(Date.now())
let timer: number | undefined
let poll: number | undefined

const keys = computed(() => pool.value?.keys || [])

function resetIn(k: ApiKey) {
  return countdown(k.quota_reset_at, now.value)
}

const accounts = computed(() => {
  const enabled = keys.value.filter((k) => k.enabled)
  const map = new Map<string, ApiKey[]>()
  enabled.forEach((k) => map.set(k.account_login || t('keys.probed_not'), [...(map.get(k.account_login || t('keys.probed_not')) || []), k]))
  return [...map.entries()]
    .map(([login, list]) => ({
      login,
      list,
      remaining: list.reduce((s, k) => s + k.quota_remaining, 0),
      limit: list.reduce((s, k) => s + k.quota_limit, 0),
    }))
    .sort((a, b) => a.login.localeCompare(b.login))
})

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    pool.value = await keyApi.quota()
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', token: '' })
  modal.value = true
}

function openToken(row: ApiKey) {
  editing.value = row
  Object.assign(form, { name: row.name, token: '' })
  modal.value = true
}

async function save() {
  if (!form.name) {
    message.warning(t('keys.need_name'))
    return
  }
  if (editing.value) {
    await keyApi.update(editing.value.id, form.token ? { name: form.name, token: form.token } : { name: form.name })
  } else {
    if (!form.token) {
      message.warning(t('keys.need_token'))
      return
    }
    await keyApi.create({ name: form.name, token: form.token })
  }
  message.success(t('keys.saved'))
  modal.value = false
  await load()
}

async function check(row: ApiKey) {
  const res = await keyApi.check(row.id)
  if (res.last_error) message.warning(`${row.name}: ${res.last_error}`)
  else message.success(`${row.name}: ${t('keys.account_label', { v: res.account_login })} ${res.quota_remaining}/${res.quota_limit}`)
  await load(true)
}

async function checkAll() {
  const res = await keyApi.checkAll()
  message.success(t('keys.probe_count', { n: res.checked }))
  await load(true)
}

async function toggle(row: ApiKey, enabled: boolean) {
  await keyApi.update(row.id, { enabled })
  await load(true)
}

function remove(row: ApiKey) {
  dialog.warning({
    title: t('keys.delete_title'),
    content: t('keys.remove_confirm', { name: row.name }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await keyApi.remove(row.id)
      message.success(t('keys.removed'))
      await load()
    },
  })
}

onMounted(async () => {
  await load()
  timer = window.setInterval(() => {
    now.value = Date.now()
  }, 1000)
  poll = window.setInterval(() => load(true), 5000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  if (poll) clearInterval(poll)
})
</script>

<template>
  <div class="page">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('keys.desc') }}</p>
      <NSpace>
        <NButton ghost @click="checkAll">{{ t('keys.probe_all') }}</NButton>
        <NButton type="primary" ghost @click="openCreate">{{ t('keys.new') }}</NButton>
      </NSpace>
    </div>

    <NAlert type="warning" :bordered="false" style="margin-bottom: 14px">{{ t('keys.not_stack_tip') }}</NAlert>

    <div class="grid-cards">
      <StatCard :label="t('keys.pool_total')" :value="num(pool?.pool.remaining)" :desc="t('keys.rate_window', { v: num(pool?.pool.limit) })" />
      <StatCard :label="t('keys.accounts')" :value="num(pool?.pool.accounts)" accent="#52c41a" :desc="t('keys.quota_per_account')" />
      <StatCard :label="t('keys.rate')" :value="num((pool?.pool.accounts || 0) * 10)" :unit="t('keys.per_minute')" accent="#40a9ff" />
      <StatCard :label="t('keys.snapshot_at')" :value="relTime(pool?.updated_at)" accent="#8b9bb4" :desc="dateTime(pool?.updated_at)" />
    </div>

    <NCard v-for="a in accounts" :key="a.login" size="small" :title="t('keys.account_label', { v: a.login })" style="margin-top: 14px">
      <template #header-extra>
        <span class="mono" style="color: #8b9bb4">{{ t('keys.quota_shared', { v: `${a.remaining}/${a.limit}` }) }}</span>
      </template>
      <div class="keys">
        <div v-for="k in keys.filter((x) => (x.account_login || t('keys.probed_not')) === a.login && x.enabled)" :key="k.id" class="key">
          <QuotaRing :remaining="k.quota_remaining" :limit="k.quota_limit" :size="72" />
          <div class="meta">
            <div class="row" style="gap: 8px">
              <b>{{ k.name }}</b>
              <StatusTag :status="k.status" />
              <span class="mono" style="color: #4a5568">{{ k.token_hint }}</span>
            </div>
            <div class="mono line">{{ t('keys.reset_countdown') }}{{ resetIn(k) }} · {{ t('keys.total_used') }}{{ num(k.total_used) }} · {{ t('keys.last_used') }}{{ relTime(k.last_used_at) }}</div>
            <div class="mono line" :style="{ color: k.last_error ? '#ff4d4f' : '#4a5568' }">
              {{ k.last_error || `${t('keys.snapshot_source', { v: k.quota_source })} · ${relTime(k.quota_updated_at)}` }}
            </div>
            <div class="row" style="gap: 6px; margin-top: 6px; flex-wrap: wrap; justify-content: flex-end">
              <NButton size="tiny" quaternary type="primary" @click="check(k)">{{ t('keys.probe') }}</NButton>
              <NButton size="tiny" quaternary @click="openToken(k)">{{ t('common.edit') }}</NButton>
              <NButton size="tiny" quaternary type="warning" @click="toggle(k, false)">{{ t('keys.disable') }}</NButton>
              <NButton size="tiny" quaternary type="error" @click="remove(k)">{{ t('common.delete') }}</NButton>
            </div>
          </div>
        </div>
      </div>
    </NCard>

    <NCard v-if="keys.some((k) => !k.enabled)" size="small" :title="t('common.disabled')" style="margin-top: 14px">
      <div v-for="k in keys.filter((x) => !x.enabled)" :key="k.id" class="row" style="justify-content: space-between; padding: 6px 0">
        <span class="mono">{{ k.name }} {{ k.token_hint }} · {{ k.account_login || t('keys.probed_not') }}</span>
        <NButton size="tiny" quaternary type="primary" @click="toggle(k, true)">{{ t('keys.enable') }}</NButton>
      </div>
    </NCard>

    <NModal v-model:show="modal" preset="card" :title="editing ? t('keys.edit', { name: editing.name }) : t('keys.new')" style="width: 480px">
      <NForm label-placement="top">
        <NFormItem :label="t('keys.name')" required>
          <NInput v-model:value="form.name" :placeholder="t('keys.name_placeholder')" />
        </NFormItem>
        <NFormItem :label="editing ? t('keys.token_new') : 'GitHub Token'" required>
          <NInput v-model:value="form.token" type="password" show-password-on="click" placeholder="ghp_xxx / github_pat_xxx" />
        </NFormItem>
      </NForm>
      <NAlert type="info" :bordered="false">
        {{ t('keys.token_tip') }}
      </NAlert>
      <template #footer>
        <NButton type="primary" @click="save">{{ t('common.save') }}</NButton>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.keys {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 12px;
}
.key {
  display: flex;
  gap: 14px;
  padding: 12px;
  border: 1px solid rgba(0, 229, 255, 0.15);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.2);
}
.meta {
  flex: 1;
  min-width: 0;
}
.line {
  font-size: 11.5px;
  color: #8b9bb4;
  margin-top: 4px;
  overflow-wrap: anywhere;
}
</style>
