<script setup lang="ts">
import { t } from '@/i18n'
import { h, onMounted, reactive, ref } from 'vue'
import { NButton, NCard, NDataTable, NForm, NFormItem, NInput, NModal, NPopconfirm, NRadioButton, NRadioGroup, NSwitch, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { userApi } from '@/api/endpoints'
import type { User } from '@/api/types'
import { dateTime, relTime } from '@/utils/format'

const message = useMessage()
const loading = ref(false)
const rows = ref<User[]>([])
const modal = ref(false)
const editing = ref<number | null>(null)
const form = reactive({ username: '', nickname: '', role: 'viewer' as 'admin' | 'viewer', password: '' })

async function load() {
  loading.value = true
  try {
    rows.value = (await userApi.list({ page: 1, page_size: 100 })).items
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { username: '', nickname: '', role: 'viewer', password: '' })
  modal.value = true
}

function openEdit(row: User) {
  editing.value = row.id
  Object.assign(form, { username: row.username, nickname: row.nickname, role: row.role, password: '' })
  modal.value = true
}

async function save() {
  if (!form.username) {
    message.warning(t('users.need_name'))
    return
  }
  if (editing.value) await userApi.update(editing.value, { nickname: form.nickname, role: form.role })
  else {
    if (form.password.length < 6) {
      message.warning(t('users.pwd_min'))
      return
    }
    await userApi.create({ ...form })
  }
  message.success(t('common.applied'))
  modal.value = false
  await load()
}

async function toggleStatus(row: User, v: boolean) {
  await userApi.setStatus(row.id, v ? 1 : 0)
  await load()
}

async function resetPassword(row: User) {
  await userApi.resetPassword(row.id, 'graycrow@123')
  message.success(t('users.reset_pwd_done', { name: row.username }))
}

async function resetTotp(row: User) {
  await userApi.resetTotp(row.id)
  message.success(t('users.reset_totp_done', { name: row.username }))
  await load()
}

const columns: DataTableColumns<User> = [
  { title: t('users.username'), key: 'username', width: 140, render: (row) => h('span', { class: 'mono' }, row.username) },
  { title: t('users.nickname'), key: 'nickname', width: 160 },
  { title: t('users.role'), key: 'role', width: 110, render: (row) => h(NTag, { size: 'small', type: row.role === 'admin' ? 'warning' : 'default', bordered: false }, { default: () => (row.role === 'admin' ? t('role.admin') : t('role.viewer')) }) },
  { title: t('users.last_login'), key: 'last_login_at', width: 150, render: (row) => (row.last_login_at ? relTime(row.last_login_at) : t('common.never')) },
  { title: t('users.created_at'), key: 'created_at', width: 170, render: (row) => dateTime(row.created_at) },
  { title: t('users.totp'), key: 'totp_enabled', width: 100, render: (row) => h(NTag, { size: 'small', bordered: false, type: row.totp_enabled ? 'success' : 'default' }, { default: () => (row.totp_enabled ? t('users.totp_on') : t('users.totp_off')) }) },
  { title: t('common.enabled'), key: 'status', width: 80, render: (row) => h(NSwitch, { value: row.status === 1, size: 'small', onUpdateValue: (v: boolean) => toggleStatus(row, v) }) },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 240,
    render: (row) =>
      h('div', { class: 'row', style: 'gap:6px' }, [
        h(NButton, { size: 'tiny', quaternary: true, onClick: () => openEdit(row) }, { default: () => t('common.edit') }),
        h(
          NPopconfirm,
          { onPositiveClick: () => resetPassword(row) },
          { trigger: () => h(NButton, { size: 'tiny', quaternary: true, type: 'warning' }, { default: () => t('users.reset_pwd') }), default: () => t('users.reset_pwd_confirm') },
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => resetTotp(row) },
          { trigger: () => h(NButton, { size: 'tiny', quaternary: true, disabled: !row.totp_enabled }, { default: () => t('users.reset_totp') }), default: () => t('users.reset_totp_confirm') },
        ),
      ]),
  },
]

onMounted(load)
</script>

<template>
  <div class="page page-fill">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('users.desc') }}</p>
      <NButton type="primary" ghost @click="openCreate">{{ t('users.new') }}</NButton>
    </div>

    <NCard size="small" class="table-fill">
      <NDataTable flex-height style="flex: 1; min-height: 0" :loading="loading" :columns="columns" :data="rows" :row-key="(r: User) => r.id" />
    </NCard>

    <NModal v-model:show="modal" preset="card" :title="editing ? t('users.edit') : t('users.new')" style="width: 440px">
      <NForm label-placement="top">
        <NFormItem :label="t('users.username')" required>
          <NInput v-model:value="form.username" :disabled="!!editing" />
        </NFormItem>
        <NFormItem :label="t('users.nickname')">
          <NInput v-model:value="form.nickname" />
        </NFormItem>
        <NFormItem :label="t('users.role')">
          <NRadioGroup v-model:value="form.role">
            <NRadioButton value="admin">{{ t('role.admin') }}</NRadioButton>
            <NRadioButton value="viewer">{{ t('role.viewer') }}</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem v-if="!editing" :label="t('users.initial_pwd')" required>
          <NInput v-model:value="form.password" type="password" show-password-on="click" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NButton type="primary" @click="save">{{ t('common.save') }}</NButton>
      </template>
    </NModal>
  </div>
</template>
