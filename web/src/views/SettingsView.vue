<script setup lang="ts">
import { t } from '@/i18n'
import { onMounted, reactive, ref } from 'vue'
import { NButton, NCard, NForm, NFormItem, NInputNumber, NSpin, useMessage } from 'naive-ui'
import { settingsApi } from '@/api/endpoints'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const form = reactive({
  default_interval_sec: 3600 as number | null,
  dry_run_days: 3 as number | null,
})

onMounted(async () => {
  loading.value = true
  try {
    Object.assign(form, await settingsApi.get())
  } finally {
    loading.value = false
  }
})

async function save() {
  if ((form.default_interval_sec || 0) < 60) {
    message.warning(t('settings.interval_min'))
    return
  }
  saving.value = true
  try {
    await settingsApi.update({ ...form, default_interval_sec: form.default_interval_sec || 3600, dry_run_days: form.dry_run_days || 0 })
    message.success(t('common.applied'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page">
    <p class="page-desc">{{ t('settings.desc') }}</p>

    <NCard size="small" style="max-width: 620px">
      <NSpin :show="loading">
        <NForm label-placement="left" label-width="150">
          <NFormItem :label="t('settings.interval')">
            <NInputNumber v-model:value="form.default_interval_sec" :min="60" :step="60" style="width: 100%" />
          </NFormItem>
          <NFormItem :label="t('settings.dry_days')">
            <NInputNumber v-model:value="form.dry_run_days" :min="0" style="width: 100%" />
          </NFormItem>
          <NFormItem label=" ">
            <NButton type="primary" :loading="saving" @click="save">{{ t('settings.save') }}</NButton>
          </NFormItem>
        </NForm>
      </NSpin>
    </NCard>
  </div>
</template>
