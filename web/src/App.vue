<script setup lang="ts">
import { computed } from 'vue'
import { darkTheme, dateEnUS, dateZhCN, enUS, NConfigProvider, NDialogProvider, NLoadingBarProvider, NMessageProvider, zhCN } from 'naive-ui'
import { locale } from '@/i18n'
import { themeOverrides } from '@/styles/theme'

const uiLocale = computed(() => (locale.value === 'zh' ? zhCN : enUS))
const uiDateLocale = computed(() => (locale.value === 'zh' ? dateZhCN : dateEnUS))
</script>

<template>
  <NConfigProvider
    :theme="darkTheme"
    :theme-overrides="themeOverrides"
    :locale="uiLocale"
    :date-locale="uiDateLocale"
  >
    <NLoadingBarProvider>
      <NDialogProvider>
        <NMessageProvider>
          <RouterView />
        </NMessageProvider>
      </NDialogProvider>
    </NLoadingBarProvider>
  </NConfigProvider>
</template>
