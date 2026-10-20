<script setup lang="ts">
import { t } from '@/i18n'
import { onMounted, reactive, ref } from 'vue'
import { NButton, NCard, NForm, NFormItem, NInput, NModal, NSpace, NSpin, NTag, useDialog, useMessage } from 'naive-ui'
import { templateApi } from '@/api/endpoints'
import type { NotifyTemplate } from '@/api/types'
import { dateTime } from '@/utils/format'
import EmptyHint from '@/components/EmptyHint.vue'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const rows = ref<NotifyTemplate[]>([])
const modal = ref(false)
const editing = ref<NotifyTemplate | null>(null)
const form = reactive({ name: '', title: '', body: '' })

async function load() {
  loading.value = true
  try {
    rows.value = await templateApi.list()
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', title: '【灰鸦告警汇总】{{title}}：本次扫描发现 {{count}} 条新问题', body: defaultBody() })
  modal.value = true
}

function defaultBody() {
  return '【灰鸦告警汇总】{{title}}：本次扫描发现 {{count}} 条新问题\n仓库：{{repos}}\n{{items}}'
}

function openEdit(row: NotifyTemplate) {
  editing.value = row
  Object.assign(form, { name: row.name, title: row.title, body: row.body })
  modal.value = true
}

async function save() {
  if (!form.name.trim()) {
    message.warning(t('templates.need_name'))
    return
  }
  if (!form.body.trim()) {
    message.warning(t('templates.need_body'))
    return
  }
  try {
    if (editing.value) await templateApi.update(editing.value.id, form)
    else await templateApi.create(form)
    message.success(t('common.applied'))
    modal.value = false
    await load()
  } catch (e) {
    message.error((e as Error).message)
  }
}

function remove(row: NotifyTemplate) {
  dialog.warning({
    title: t('templates.delete'),
    content: t('templates.delete_confirm', { name: row.name }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await templateApi.remove(row.id)
        message.success(t('common.applied'))
        await load()
      } catch (e) {
        message.error((e as Error).message)
      }
    },
  })
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('templates.desc') }}</p>
      <NButton type="primary" ghost @click="openCreate">{{ t('templates.new') }}</NButton>
    </div>

    <NCard size="small">
      <NSpin :show="loading">
        <div class="list">
          <div v-for="row in rows" :key="row.id" class="item">
            <div class="left">
              <b>{{ row.name }}</b>
              <NTag v-if="row.is_default" size="small" type="info" :bordered="false">{{ t('templates.default') }}</NTag>
              <div class="mono sub">{{ row.title || t('templates.no_title') }}</div>
              <div class="mono sub">{{ t('templates.updated') }} {{ dateTime(row.updated_at) }}</div>
            </div>
            <NSpace>
              <NButton size="tiny" quaternary @click="openEdit(row)">{{ t('common.edit') }}</NButton>
              <NButton v-if="!row.is_default" size="tiny" quaternary type="error" @click="remove(row)">{{ t('common.delete') }}</NButton>
            </NSpace>
          </div>
        </div>
        <EmptyHint v-if="!rows.length" :text="t('templates.empty')" :action="t('templates.new')" @action="openCreate" />
      </NSpin>
    </NCard>

    <NModal v-model:show="modal" preset="card" :title="editing ? t('templates.edit') : t('templates.new')" style="width: 680px">
      <NForm label-placement="top">
        <NFormItem :label="t('templates.name')" required>
          <NInput v-model:value="form.name" :placeholder="t('templates.name_placeholder')" />
        </NFormItem>
        <NFormItem :label="t('templates.title')">
          <NInput v-model:value="form.title" :placeholder="t('templates.title_placeholder')" />
        </NFormItem>
        <NFormItem :label="t('templates.body')" required>
          <NInput v-model:value="form.body" type="textarea" :rows="10" class="mono-input" />
        </NFormItem>
        <div class="hint">{{ t('templates.vars') }}</div>
      </NForm>
      <template #footer>
        <NButton type="primary" @click="save">{{ t('common.save') }}</NButton>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.list {
  display: flex;
  flex-direction: column;
}
.item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 4px;
  border-bottom: 1px dashed rgba(0, 229, 255, 0.1);
}
.item:last-child {
  border-bottom: none;
}
.sub {
  color: #4a5568;
  font-size: 11.5px;
  margin-top: 3px;
  white-space: pre-wrap;
}
.hint {
  color: #7a8ba5;
  font-size: 12px;
}
.mono-input :deep(textarea) {
  font-family: 'JetBrains Mono', Menlo, Consolas, monospace;
  font-size: 12.5px;
}
</style>
