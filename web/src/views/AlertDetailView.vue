<script setup lang="ts">
import { t } from '@/i18n'
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NCard, NDatePicker, NForm, NFormItem, NInput, NRadioButton, NRadioGroup, NSpace, NSpin, NTimeline, NTimelineItem, useDialog, useMessage } from 'naive-ui'
import { alertApi } from '@/api/endpoints'
import type { Alert, AlertStatus } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { dateTime, relTime } from '@/utils/format'
import SnippetViewer from '@/components/SnippetViewer.vue'
import StatusTag from '@/components/StatusTag.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()

const id = Number(route.params.id)
const alert = ref<Alert | null>(null)
const note = ref('')
const loading = ref(false)
const allowForm = ref({ scope: 'global' as 'global' | 'rule', reason: '', expires_at: null as string | null })

// 存 key 由 t() 渲染：写死文案切语言不生效
const actionLabels: Record<string, string> = {
  create: 'detail.discovered',
  status_change: 'detail.status_event',
  note: 'detail.note_short',
  allowlist: 'detail.manual_allow',
  notify: 'detail.notify_event',
}
const actionLabel = (action: string) => (actionLabels[action] ? t(actionLabels[action]) : action)

async function load() {
  loading.value = true
  try {
    alert.value = await alertApi.detail(id)
  } finally {
    loading.value = false
  }
}

async function setStatus(status: AlertStatus) {
  if (!alert.value) return
  await alertApi.setStatus(id, status, note.value)
  message.success(t('detail.status_changed'))
  note.value = ''
  await load()
}

function allowlist() {  dialog.warning({
    title: t('detail.allow_title'),
    content: `${t('detail.entry_prefix')}${alert.value?.repo_full_name}${t('detail.dry_run_only')}`,
    positiveText: t('detail.allow_confirm'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await alertApi.allowlist(id, allowForm.value.scope, allowForm.value.reason || t('detail.manual_allow'), allowForm.value.expires_at || undefined)
      message.success(t('status.allowlisted'))
      await load()
    },
  })
}

function removeAlert() {
  dialog.warning({
    title: t('common.delete'),
    content: t('alerts.confirm_delete', { n: 1 }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await alertApi.remove(id)
      message.success(t('alerts.deleted', { n: 1 }))
      router.push('/alerts')
    },
  })
}

onMounted(load)

function openLink(url: string) {
  window.open(url, '_blank', 'noopener')
}
</script>

<template>
  <div class="page" v-if="alert">
    <div class="row" style="justify-content: space-between; align-items: flex-start">
      <div>
        <h2 class="page-title">
          {{ alert.repo_full_name }}
          <span class="mono" style="color: #8b9bb4; font-size: 13px">{{ alert.path }}</span>
        </h2>
        <p class="page-desc">{{ t('nav.rules') }}<em>{{ alert.rule_name }}</em> · {{ t('detail.first_prefix') }}{{ dateTime(alert.first_seen_at) }} · {{ t('detail.last_prefix') }}{{ relTime(alert.last_seen_at) }}
        </p>
      </div>
      <div class="row" style="gap: 8px">
        <StatusTag :status="alert.status" />
        <NButton v-if="auth.isAdmin" quaternary type="error" @click="removeAlert">{{ t('common.delete') }}</NButton>
        <NButton quaternary @click="router.back()">{{ t('detail.back') }}</NButton>
      </div>
    </div>

    <div class="cols">
      <div class="left">
        <NCard size="small" :title="t('detail.snippet')">
          <SnippetViewer :snippet="alert.snippet" :keyword="alert.matched_keyword" />
          <div class="row" style="gap: 10px; margin-top: 12px">
            <NButton size="small" type="primary" ghost @click="openLink(alert.html_url)">{{ t('detail.file') }}</NButton>
            <NButton size="small" quaternary @click="openLink(alert.repo_url)">{{ t('detail.repo_home') }}</NButton>
          </div>
        </NCard>

        <NCard size="small" :title="t('detail.action')" style="margin-top: 14px">
          <NInput v-model:value="note" type="textarea" :rows="2" :placeholder="t('detail.note')" style="margin-bottom: 12px" />
          <NSpace>
            <NButton type="success" size="small" @click="setStatus('resolved')">{{ t('detail.mark_resolved') }}</NButton>
            <NButton type="info" size="small" @click="setStatus('false_positive')">{{ t('detail.mark_fp') }}</NButton>
            <NButton size="small" @click="setStatus('ignored')">{{ t('detail.ignore') }}</NButton>
            <NButton type="warning" size="small" ghost @click="allowlist">{{ t('detail.allow_one_click') }}</NButton>
          </NSpace>
        </NCard>

        <NCard size="small" :title="t('detail.allow_settings')" style="margin-top: 14px">
          <NForm label-placement="left" :show-feedback="false">
            <NFormItem :label="t('detail.allow_scope')">
              <NRadioGroup v-model:value="allowForm.scope" size="small">
                <NRadioButton value="global">{{ t('detail.allow_global') }}</NRadioButton>
                <NRadioButton value="rule">{{ t('detail.allow_rule') }}</NRadioButton>
              </NRadioGroup>
            </NFormItem>
            <NFormItem :label="t('detail.allow_reason')">
              <NInput v-model:value="allowForm.reason" size="small" :placeholder="t('detail.allow_reason')" />
            </NFormItem>
            <NFormItem :label="t('detail.allow_expires')">
              <NDatePicker v-model:formatted-value="allowForm.expires_at" type="datetime" size="small" clearable value-format="yyyy-MM-dd'T'HH:mm:ss'Z'" />
            </NFormItem>
          </NForm>
        </NCard>
      </div>

      <div class="right">
        <NCard size="small" :title="t('detail.timeline')">
          <template #header-extra>
            <NButton size="tiny" ghost type="primary" @click="alertApi.renotify(id).then(() => message.success(t('detail.renotified')))">{{ t('detail.renotify') }}</NButton>
          </template>
          <NTimeline>
            <NTimelineItem
              v-for="e in alert.events || []"
              :key="e.id"
              :type="e.action === 'notify' ? 'info' : e.action === 'create' ? 'warning' : 'success'"
              :title="`${e.actor_name} · ${actionLabel(e.action)}`"
              :time="dateTime(e.created_at)"
            >
              {{ e.note || '-' }}
            </NTimelineItem>
          </NTimeline>
        </NCard>

      </div>
    </div>
  </div>
  <NSpin v-else :show="loading" style="display: block; margin: 80px auto" />
</template>

<style scoped>
.cols {
  display: grid;
  grid-template-columns: 1.5fr 1fr;
  gap: 14px;
}
@media (max-width: 1100px) {
  .cols {
    grid-template-columns: 1fr;
  }
}
.page-title em {
  font-style: normal;
  color: #00e5ff;
}
.log {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px dashed rgba(0, 229, 255, 0.1);
  font-size: 12.5px;
}
.preview {
  margin: 0;
  padding: 10px 12px;
  background: rgba(0, 0, 0, 0.3);
  border-radius: 8px;
  color: #c3d3e8;
  white-space: pre-wrap;
  line-height: 1.7;
}
</style>
