<script setup lang="ts">
import { t } from '@/i18n'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NCard, NForm, NFormItem, NInput, NInputNumber, NModal, NSelect, NSpace, NSpin, NSwitch, NTag, useDialog, useMessage } from 'naive-ui'
import { channelApi, templateApi } from '@/api/endpoints'
import type { Channel, ChannelType, NotifyTemplate } from '@/api/types'
import { relTime } from '@/utils/format'
import EmptyHint from '@/components/EmptyHint.vue'

const message = useMessage()
const dialog = useDialog()
const router = useRouter()
const loading = ref(false)
const rows = ref<Channel[]>([])
const templates = ref<NotifyTemplate[]>([])
const modal = ref(false)
const editing = ref<Channel | null>(null)
const form = reactive({
  name: '',
  mode: 'webhook' as 'webhook' | 'smtp',
  type: 'webhook' as ChannelType,
  template_id: 0 as number | null,
  enabled: true,
  url: '',
  url_hint: '',
  body_format: 'json',
  sign: false,
  sign_secret: '',
  headers: '',
  host: '',
  port: 587 as number | null,
  from: '',
  to: '',
  password: '',
})

const modeOptions = [
  { label: t('channels.mode_webhook'), value: 'webhook' },
  { label: t('channels.mode_smtp'), value: 'smtp' },
]
const platformOptions = [
  { label: t('channels.types.wecom'), value: 'wecom' },
  { label: t('channels.types.dingtalk'), value: 'dingtalk' },
  { label: t('channels.types.feishu'), value: 'feishu' },
  { label: t('channels.types.webhook'), value: 'webhook' },
]
const typeLabel = computed<Record<string, string>>(() =>
  Object.fromEntries(['feishu', 'wecom', 'dingtalk', 'webhook', 'smtp'].map((v) => [v, t(`channels.types.${v}`)])),
)
const formatOptions = computed(() => [
  ...(form.type === 'webhook' ? [{ label: t('channels.format_json'), value: 'json' }] : []),
  { label: t('channels.format_text'), value: 'text' },
  { label: t('channels.format_markdown'), value: 'markdown' },
])
const canSign = computed(() => form.type === 'feishu' || form.type === 'dingtalk')
const templateOptions = computed(() => templates.value.map((tpl) => ({ label: tpl.name, value: tpl.id })))
const defaultTemplateId = computed(() => templates.value.find((tpl) => tpl.is_default)?.id ?? null)

function setMode(value: 'webhook' | 'smtp') {
  form.mode = value
  if (value === 'smtp') {
    form.type = 'smtp'
    return
  }
  if (form.type === 'smtp') form.type = 'webhook'
  form.body_format = form.type === 'webhook' ? 'json' : 'text'
}

function setType(value: ChannelType) {
  form.type = value
  form.body_format = value === 'webhook' ? 'json' : 'text'
  form.sign = false
}

function addr(c: Channel) {
  const cfg = c.config as Record<string, unknown>
  return String(cfg.url_hint || cfg.url || cfg.webhook || cfg.host || '-')
}

async function load() {
  loading.value = true
  try {
    ;[rows.value, templates.value] = await Promise.all([channelApi.list(), templateApi.list()])
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  Object.assign(form, {
    name: '', mode: 'webhook', type: 'webhook', template_id: defaultTemplateId.value, enabled: true,
    url: '', url_hint: '', body_format: 'json', sign: false, sign_secret: '', headers: '',
    host: '', port: 587, from: '', to: '', password: '',
  })
  modal.value = true
}

function openEdit(row: Channel) {
  editing.value = row
  const c = row.config as Record<string, unknown>
  const isSmtp = row.type === 'smtp'
  Object.assign(form, {
    name: row.name,
    mode: isSmtp ? 'smtp' : 'webhook',
    type: row.type,
    template_id: row.template_id ?? defaultTemplateId.value,
    enabled: row.enabled,
    url: '',
    url_hint: String(c.url_hint || c.url || c.webhook || ''),
    body_format: isSmtp ? 'text' : String(c.body_format || (row.type === 'webhook' ? 'json' : 'text')),
    sign: !isSmtp && String(c.sign_mode || '') === row.type,
    sign_secret: '',
    headers: '',
    host: String(c.host || ''),
    port: Number(c.port || 587),
    from: String(c.from || ''),
    to: Array.isArray(c.to) ? (c.to as string[]).join(',') : String(c.to || ''),
    password: '',
  })
  modal.value = true
}

function buildConfig() {
  if (form.type === 'smtp') {
    return { host: form.host, port: form.port || 587, from: form.from, to: form.to.split(',').map((s) => s.trim()).filter(Boolean) }
  }
  const cfg: Record<string, unknown> = { body_format: form.body_format }
  if (form.sign && canSign.value) cfg.sign_mode = form.type
  if (form.url) cfg.url = form.url
  return cfg
}

function buildPayload(): Partial<Channel> & { secret?: string; headers?: string } {
  const payload: Partial<Channel> & { secret?: string; headers?: string } = {
    name: form.name,
    type: form.type,
    template_id: form.template_id || undefined,
    enabled: form.enabled,
    config: buildConfig(),
  }
  if (editing.value) payload.id = editing.value.id
  const secret = form.type === 'smtp' ? form.password : form.sign && canSign.value ? form.sign_secret : ''
  if (secret) payload.secret = secret
  if (form.type === 'webhook' && form.headers.trim()) payload.headers = form.headers.trim()
  return payload
}

async function save() {
  if (!form.name) {
    message.warning(t('channels.need_name'))
    return
  }
  if (form.type !== 'smtp' && !form.url && !form.url_hint) {
    message.warning(t('channels.need_url'))
    return
  }
  if (form.sign && canSign.value && !form.sign_secret && !editing.value) {
    message.warning(t('channels.need_sign_secret'))
    return
  }
  if (form.type === 'smtp' && (!form.host || !form.from || !form.to)) {
    message.warning(t('channels.need_smtp'))
    return
  }
  try {
    const payload = buildPayload()
    if (editing.value) await channelApi.update(editing.value.id, payload)
    else await channelApi.create(payload)
    message.success(t('common.applied'))
    modal.value = false
    await load()
  } catch (e) {
    message.error((e as Error).message)
  }
}

async function test() {
  try {
    showResult(await channelApi.testDraft(buildPayload()))
  } catch (e) {
    dialog.error({ title: t('channels.test_result'), content: (e as Error).message })
  }
}

async function testRow(row: Channel) {
  try {
    showResult(await channelApi.test(row.id))
  } catch (e) {
    dialog.error({ title: t('channels.test_result'), content: (e as Error).message })
  }
}

function showResult(res: { ok: boolean; response: string }) {
  const opts = { title: t('channels.test_result'), content: res.response || (res.ok ? 'OK' : t('channels.test_fail')) }
  if (res.ok) dialog.success(opts)
  else dialog.error(opts)
}

async function toggle(row: Channel, enabled: boolean) {
  await channelApi.update(row.id, { enabled })
  await load()
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="row" style="justify-content: space-between">
      <p class="page-desc">{{ t('channels.desc') }}</p>
      <NButton type="primary" ghost @click="openCreate">{{ t('channels.new') }}</NButton>
    </div>

    <NCard size="small">
      <NSpin :show="loading">
        <div class="list">
          <div v-for="c in rows" :key="c.id" class="item">
            <div class="left">
              <div class="row" style="gap: 8px">
                <NTag size="small" :bordered="false" type="info">{{ typeLabel[c.type] }}</NTag>
                <b>{{ c.name }}</b>
                <NTag v-if="c.config.sign_mode" size="small" :bordered="false">{{ t('channels.signed') }}</NTag>
                <NTag v-if="c.template_id" size="small" :bordered="false">{{ templates.find((x) => x.id === c.template_id)?.name }}</NTag>
                <NTag v-if="!c.enabled" size="small" :bordered="false">{{ t('status.disabled') }}</NTag>
                <NTag v-else-if="c.last_status === 'failed'" size="small" type="error" :bordered="false">{{ t('channels.last_fail') }}</NTag>
                <NTag v-else-if="c.last_status" size="small" type="success" :bordered="false">{{ t('channels.last_ok') }}</NTag>
              </div>
              <div class="mono sub">
                {{ addr(c) }}
                <span v-if="c.last_sent_at"> · {{ relTime(c.last_sent_at) }}</span>
              </div>
              <div v-if="c.last_error" class="mono" style="color: #ff4d4f; font-size: 11.5px">{{ c.last_error }}</div>
            </div>
            <NSpace>
              <NButton size="tiny" quaternary @click="openEdit(c)">{{ t('common.edit') }}</NButton>
              <NButton size="tiny" quaternary type="primary" @click="testRow(c)">{{ t('channels.test') }}</NButton>
              <NSwitch :value="c.enabled" size="small" @update:value="(v: boolean) => toggle(c, v)" />
            </NSpace>
          </div>
        </div>
        <EmptyHint v-if="!rows.length" :text="t('channels.empty')" :action="t('channels.new')" @action="openCreate" />
      </NSpin>
    </NCard>

    <NModal v-model:show="modal" preset="card" :title="editing ? t('channels.edit') : t('channels.new')" style="width: 560px">
      <div class="modal-body">
      <NForm label-placement="top">
        <NFormItem :label="t('channels.name')" required>
          <NInput v-model:value="form.name" :placeholder="t('channels.name_placeholder')" />
        </NFormItem>
        <NFormItem :label="t('channels.mode')">
          <NSelect :value="form.mode" :options="modeOptions" @update:value="setMode" />
        </NFormItem>
        <NFormItem v-if="form.mode === 'webhook'" :label="t('channels.type')">
          <NSelect :value="form.type" :options="platformOptions" @update:value="setType" />
        </NFormItem>
        <NFormItem :label="t('channels.template')">
          <NSpace align="center">
            <NSelect v-model:value="form.template_id" :options="templateOptions" style="min-width: 240px" />
            <NButton quaternary type="primary" @click="router.push('/templates')">{{ t('channels.manage_templates') }}</NButton>
          </NSpace>
        </NFormItem>

        <template v-if="form.mode === 'smtp'">
          <NFormItem :label="t('channels.smtp_host_label')">
            <NInput v-model:value="form.host" placeholder="smtp.example.com" />
          </NFormItem>
          <NFormItem :label="t('channels.smtp_port')">
            <NInputNumber v-model:value="form.port" style="width: 100%" />
          </NFormItem>
          <NFormItem :label="t('channels.smtp_from')">
            <NInput v-model:value="form.from" placeholder="graycrow@example.com" />
          </NFormItem>
          <NFormItem :label="t('channels.smtp_to')">
            <NInput v-model:value="form.to" placeholder="sec@example.com,ops@example.com" />
          </NFormItem>
          <NFormItem :label="t('channels.smtp_password')">
            <NInput v-model:value="form.password" type="password" show-password-on="click" :placeholder="t('channels.secret_placeholder')" />
          </NFormItem>
        </template>

        <template v-else>
          <NFormItem :label="t('channels.url')">
            <NInput v-model:value="form.url" :placeholder="form.url_hint || 'https://...'" />
            <div v-if="form.url_hint" class="hint mono">{{ t('channels.url_saved') }}：{{ form.url_hint }}</div>
          </NFormItem>
          <NFormItem :label="t('channels.body_format')">
            <NSelect v-model:value="form.body_format" :options="formatOptions" />
          </NFormItem>
          <NFormItem v-if="canSign" :label="t('channels.sign')">
            <NSwitch v-model:value="form.sign" />
          </NFormItem>
          <NFormItem v-if="canSign && form.sign" :label="t('channels.sign_secret')">
            <NInput v-model:value="form.sign_secret" type="password" show-password-on="click" :placeholder="t('channels.sign_secret_placeholder')" />
          </NFormItem>
          <NFormItem v-if="form.type === 'wecom'" :label="t('channels.sign')">
            <span class="hint">{{ t('channels.sign_hint_wecom') }}</span>
          </NFormItem>
          <NFormItem v-if="form.type === 'webhook'" :label="t('channels.headers')">
            <NInput v-model:value="form.headers" type="textarea" :rows="2" placeholder='{"X-Token":"..."}' />
          </NFormItem>
        </template>

        <NFormItem :label="t('channels.enable')">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>
      </NForm>
      </div>
      <template #footer>
        <div class="row" style="justify-content: space-between">
          <NButton quaternary type="primary" @click="test">{{ t('channels.test') }}</NButton>
          <NButton type="primary" @click="save">{{ t('common.save') }}</NButton>
        </div>
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
}
.hint {
  margin-top: 6px;
  color: #7a8ba5;
  font-size: 12px;
}
.modal-body {
  max-height: calc(100vh - 220px);
  overflow-y: auto;
  padding-right: 6px;
}
</style>
