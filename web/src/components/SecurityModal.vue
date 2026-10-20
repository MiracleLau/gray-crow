<script setup lang="ts">
import { t } from '@/i18n'
import { ref, watch } from 'vue'
import { NAlert, NButton, NCard, NFormItem, NInput, NModal, NSpace, NTag, useMessage } from 'naive-ui'
import { authApi } from '@/api/endpoints'
import { useAuthStore } from '@/stores/auth'

const show = defineModel<boolean>('show', { required: true })
const auth = useAuthStore()
const message = useMessage()

const step = ref<'idle' | 'setup' | 'codes'>('idle')
const secret = ref('')
const otpauth = ref('')
const code = ref('')
const password = ref('')
const oldPwd = ref('')
const newPwd = ref('')
const codes = ref<string[]>([])
const loading = ref(false)

watch(show, (open) => {
  if (!open) return
  step.value = 'idle'
  code.value = ''
  password.value = ''
  oldPwd.value = ''
  newPwd.value = ''
  codes.value = []
})

async function changePassword() {
  loading.value = true
  try {
    await authApi.changePassword({ old_password: oldPwd.value, new_password: newPwd.value })
    message.success(t('security.pwd_changed'))
    show.value = false
    await auth.logout()
    location.href = '/login'
  } finally {
    loading.value = false
  }
}

async function start() {
  loading.value = true
  try {
    const res = await authApi.setupTotp()
    secret.value = res.secret
    otpauth.value = res.otpauth_url
    step.value = 'setup'
  } finally {
    loading.value = false
  }
}

async function enable() {
  loading.value = true
  try {
    const res = await authApi.enableTotp(code.value.trim())
    codes.value = res.recovery_codes
    step.value = 'codes'
    if (auth.me) auth.me.totp_enabled = true
    message.success(t('security.totp_enabled'))
  } finally {
    loading.value = false
  }
}

async function disable() {
  loading.value = true
  try {
    await authApi.disableTotp(password.value)
    if (auth.me) auth.me.totp_enabled = false
    password.value = ''
    message.success(t('security.totp_disabled'))
  } finally {
    loading.value = false
  }
}

async function copyCodes() {
  try {
    await navigator.clipboard.writeText(codes.value.join('\n'))
    message.success(t('security.recovery_copied'))
  } catch {
    message.warning(t('security.recovery_manual'))
  }
}
</script>

<template>
  <NModal v-model:show="show" :mask-closable="false">
    <NCard class="security-card" :title="t('security.title')" :bordered="false" size="small" closable @close="show = false">
      <section class="section">
        <div class="row-between"><span>{{ t('security.pwd') }}</span></div>
        <NSpace vertical :size="10">
          <NInput v-model:value="oldPwd" type="password" show-password-on="click" :placeholder="t('security.old_pwd')" />
          <NInput v-model:value="newPwd" type="password" show-password-on="click" :placeholder="t('security.new_pwd')" />
          <NButton
            size="small"
            :loading="loading"
            :disabled="!oldPwd || newPwd.length < 6"
            @click="changePassword"
          >{{ t('security.change_pwd') }}</NButton>
        </NSpace>
      </section>

      <section class="section">
        <div class="row-between">
        <span>{{ t('security.totp') }}</span>
        <NTag :type="auth.me?.totp_enabled ? 'success' : 'default'" size="small" :bordered="false">
          {{ auth.me?.totp_enabled ? t('security.totp_on') : t('security.totp_off') }}
        </NTag>
      </div>

      <template v-if="step === 'idle'">
        <p class="desc">
          {{ t('security.totp_desc') }}
        </p>
        <NButton v-if="!auth.me?.totp_enabled" type="primary" :loading="loading" @click="start">{{ t('security.totp_start') }}</NButton>
        <NSpace v-else vertical :size="10">
          <NFormItem :label="t('security.old_pwd')" :show-feedback="false">
            <NInput v-model:value="password" type="password" show-password-on="click" :placeholder="t('security.confirm_identity')" />
          </NFormItem>
          <NButton type="error" ghost :loading="loading" :disabled="!password" @click="disable">{{ t('security.totp_disable') }}</NButton>
        </NSpace>
      </template>

      <template v-else-if="step === 'setup'">
        <p class="desc">{{ t('security.totp_secret_tip') }}</p>
        <div class="secret mono">{{ secret }}</div>
        <p class="desc mono break">{{ otpauth }}</p>
        <NFormItem :label="t('security.totp_code')" :show-feedback="false">
          <NInput v-model:value="code" maxlength="6" :placeholder="t('login.totp_placeholder')" class="mono" />
        </NFormItem>
        <NButton type="primary" :loading="loading" :disabled="code.length !== 6" @click="enable">{{ t('security.totp_enable') }}</NButton>
      </template>

      <template v-else>
        <NAlert type="warning" :bordered="false" :title="t('security.recovery_title')">
          {{ t('security.recovery_desc') }}
        </NAlert>
        <div class="codes mono">
          <span v-for="c in codes" :key="c">{{ c }}</span>
        </div>
        <NButton size="small" @click="copyCodes">{{ t('security.recovery_copy') }}</NButton>
      </template>
      </section>
    </NCard>
  </NModal>
</template>

<style scoped>
.security-card {
  width: 460px;
  background: rgba(17, 23, 38, 0.96);
  border: 1px solid rgba(0, 229, 255, 0.18);
}
.row-between {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.section + .section {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid rgba(0, 229, 255, 0.12);
}
.desc {
  margin: 0 0 12px;
  color: #8b9bb4;
  font-size: 12.5px;
  line-height: 1.7;
}
.break {
  word-break: break-all;
}
.secret {
  padding: 10px 12px;
  margin-bottom: 10px;
  background: rgba(0, 229, 255, 0.06);
  border: 1px dashed rgba(0, 229, 255, 0.35);
  border-radius: 8px;
  font-size: 14px;
  letter-spacing: 2px;
  color: #00e5ff;
  user-select: all;
}
.codes {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 6px 16px;
  margin: 12px 0;
}
.codes span {
  color: #e8eef7;
  letter-spacing: 1px;
  user-select: all;
}
</style>
