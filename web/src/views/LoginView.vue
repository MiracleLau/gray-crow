<script setup lang="ts">
import { isZh, t } from '@/i18n'
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NForm, NFormItem, NInput, useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'
import { USE_MOCK } from '@/api/request'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()

const username = ref(USE_MOCK ? 'admin' : '')
const password = ref(USE_MOCK ? 'admin123' : '')
const code = ref('')
const recovery = ref(false)
const loading = ref(false)

async function submit() {
  if (!username.value || !password.value) {
    message.warning(t('login.need_input'))
    return
  }
  loading.value = true
  try {
    const needTotp = await auth.login(username.value, password.value)
    if (!needTotp) router.push((route.query.redirect as string) || '/dashboard')
  } catch (e) {
    message.error((e as Error).message || t('login.failed'))
  } finally {
    loading.value = false
  }
}

async function submitTotp() {
  if (!code.value) {
    message.warning(recovery.value ? t('login.recovery_tip') : t('login.totp_label'))
    return
  }
  loading.value = true
  try {
    await auth.verifyTotp(code.value.trim(), recovery.value)
    router.push((route.query.redirect as string) || '/dashboard')
  } catch (e) {
    message.error((e as Error).message || t('login.verify_failed'))
  } finally {
    loading.value = false
  }
}

function back() {
  auth.cancelTotp()
  code.value = ''
  recovery.value = false
}
</script>

<template>
  <div class="login">
    <div class="card glass">
      <div class="head">
        <img class="logo" src="/logo.png" alt="" />
        <h1>{{ t('app.name') }} <em v-if="isZh">GrayCrow</em></h1>
        <p>{{ t('app.slogan') }}</p>
      </div>
      <NForm v-if="!auth.pendingTicket" :show-label="true" label-placement="top" @submit.prevent>
        <NFormItem :label="t('common.account')">
          <NInput v-model:value="username" :placeholder="t('login.username')" @keyup.enter="submit" />
        </NFormItem>
        <NFormItem :label="t('common.password')">
          <NInput v-model:value="password" type="password" show-password-on="click" :placeholder="t('common.password')" @keyup.enter="submit" />
        </NFormItem>
        <NButton type="primary" block :loading="loading" attr-type="submit" @click="submit">{{ t('login.submit') }}</NButton>
      </NForm>
      <NForm v-else :show-label="true" label-placement="top" @submit.prevent>
        <p class="totp-tip">{{ recovery ? t('login.recovery_tip') : t('login.totp_tip') }}</p>
        <NFormItem :label="recovery ? t('login.recovery_label') : t('login.totp_label')">
          <NInput
            v-model:value="code"
            :maxlength="recovery ? 8 : 6"
            :placeholder="recovery ? t('login.recovery_placeholder') : t('login.totp_placeholder')"
            class="mono"
            @keyup.enter="submitTotp"
          />
        </NFormItem>
        <NButton type="primary" block :loading="loading" attr-type="submit" @click="submitTotp">{{ t('login.verify') }}</NButton>
        <div class="totp-links">
          <a @click="recovery = !recovery">{{ recovery ? t('login.use_totp') : t('login.use_recovery') }}</a>
          <a @click="back">{{ t('login.back') }}</a>
        </div>
      </NForm>
      <div v-if="USE_MOCK" class="hint mono">{{ t('login.mock_hint') }}</div>
    </div>
  </div>
</template>

<style scoped>
.login {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
.card {
  width: 372px;
  padding: 32px 30px 26px;
  background: rgba(17, 23, 38, 0.78);
  border: 1px solid rgba(0, 229, 255, 0.18);
  border-radius: 14px;
  box-shadow: 0 0 40px rgba(0, 229, 255, 0.1);
}
.glass {
  backdrop-filter: blur(10px);
}
.head {
  text-align: center;
  margin-bottom: 22px;
}
.logo {
  width: 64px;
  height: 64px;
  filter: drop-shadow(0 0 18px rgba(0, 229, 255, 0.45));
}
h1 {
  margin: 8px 0 2px;
  font-size: 21px;
  letter-spacing: 3px;
}
h1 em {
  font-style: normal;
  font-size: 12px;
  color: #8b9bb4;
  letter-spacing: 4px;
  display: block;
  margin-top: 4px;
}
p {
  margin: 0;
  color: #8b9bb4;
  font-size: 12.5px;
}
.hint {
  margin-top: 16px;
  color: #4a5568;
  font-size: 11.5px;
  text-align: center;
}
.totp-tip {
  margin: 0 0 14px;
  color: #8b9bb4;
  font-size: 12px;
  line-height: 1.6;
}
.totp-links {
  margin-top: 14px;
  display: flex;
  justify-content: space-between;
  font-size: 12px;
}
.totp-links a {
  color: #00e5ff;
  cursor: pointer;
}
.totp-links a:hover {
  text-decoration: underline;
}
</style>
