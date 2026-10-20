<script setup lang="ts">
import { locale, setLocale, t } from '@/i18n'
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { NAvatar, NBadge, NButton, NDropdown, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NMenu, NTooltip, useDialog } from 'naive-ui'
import type { DropdownOption, MenuOption } from 'naive-ui'
import SecurityModal from '@/components/SecurityModal.vue'
import { alertApi } from '@/api/endpoints'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { collapsed } = storeToRefs(useAppStore())
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const dialog = useDialog()
const securityOpen = ref(false)

const pending = ref(0)
let timer: number | undefined

// 徽标只反映「我可见的待处理告警」，与告警中心列表同源
async function refreshPending() {
  try {
    pending.value = (await alertApi.list({ status: 'pending', page: 1, page_size: 1 })).total
  } catch {
    pending.value = 0
  }
}

onMounted(() => {
  void refreshPending()
  timer = window.setInterval(() => {
    void refreshPending()
  }, 60000)
})
onBeforeUnmount(() => window.clearInterval(timer))

// computed：切换语言时菜单文案要跟着变
const ICON_PATHS: Record<string, string[]> = {
  dashboard: ['M3 3h7v9H3z', 'M14 3h7v5h-7z', 'M14 12h7v9h-7z', 'M3 16h7v5H3z'],
  alerts: ['M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9', 'M13.7 21a2 2 0 0 1-3.4 0'],
  notify: ['M22 2 11 13', 'M22 2 15 22l-4-9-9-4Z'],
  stats: ['M12 20V10', 'M18 20V4', 'M6 20v-4'],
  rules: ['M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z', 'M9 12l2 2 4-4'],
  allowlist: ['M3 17l2 2 4-4', 'M3 7l2 2 4-4', 'M13 6h8', 'M13 12h8', 'M13 18h8'],
  api_keys: ['M15.5 7.5 17.8 9.8a1 1 0 0 0 1.4 0l2.1-2.1a1 1 0 0 0 0-1.4L19 4', 'M21 2l-9.6 9.6', 'M7.5 21a5.5 5.5 0 1 0 0-11 5.5 5.5 0 0 0 0 11Z'],
  channels: ['M4.9 19.1C1 15.2 1 8.8 4.9 4.9', 'M7.8 16.2c-2.3-2.3-2.3-6.1 0-8.5', 'M12 14a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z', 'M16.2 7.8c2.3 2.3 2.3 6.1 0 8.5', 'M19.1 4.9C23 8.8 23 15.2 19.1 19.1'],
  templates: ['M4 3h16v18H4z', 'M8 8h8', 'M8 12h8', 'M8 16h5'],
  users: ['M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2', 'M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z', 'M22 21v-2a4 4 0 0 0-3-3.87', 'M16 3.13a4 4 0 0 1 0 7.75'],
  audit: ['M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z', 'M14 2v4a2 2 0 0 0 2 2h4', 'M10 9H8', 'M16 13H8', 'M16 17H8'],
  settings: ['M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z', 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z'],
}

function icon(name: keyof typeof ICON_PATHS) {
  return () =>
    h(
      'svg',
      { viewBox: '0 0 24 24', width: 18, height: 18, fill: 'none', stroke: 'currentColor', 'stroke-width': 1.8, 'stroke-linecap': 'round', 'stroke-linejoin': 'round' },
      ICON_PATHS[name].map((d) => h('path', { d })),
    )
}

const menu = computed<MenuOption[]>(() => [
  { label: t('nav.dashboard'), key: '/dashboard', icon: icon('dashboard') },
  { label: t('nav.alerts'), key: '/alerts', icon: icon('alerts') },
  { label: t('nav.notify'), key: '/notify-records', icon: icon('notify') },
  { label: t('nav.stats'), key: '/stats', icon: icon('stats') },
  { label: t('nav.rules'), key: '/rules', icon: icon('rules') },
  { label: t('nav.allowlist'), key: '/allowlist', icon: icon('allowlist') },
  { label: t('nav.api_keys'), key: '/api-keys', icon: icon('api_keys') },
  { label: t('nav.templates'), key: '/templates', icon: icon('templates') },
  { label: t('nav.channels'), key: '/channels', icon: icon('channels') },
  { label: t('nav.users'), key: '/users', icon: icon('users') },
  { label: t('nav.audit'), key: '/audit-logs', icon: icon('audit') },
  { label: t('nav.settings'), key: '/settings', icon: icon('settings') },
])

const items = computed(() => menu.value.filter((i) => auth.isAdmin || !['/rules', '/allowlist', '/api-keys', '/channels', '/templates', '/users', '/audit-logs', '/settings'].includes(String(i.key))))
const activeKey = computed(() => (route.meta.activeMenu as string) || route.path)

function onSelect(key: string) {
  router.push(key)
}

const accountOptions = computed<DropdownOption[]>(() => [
  {
    key: 'role',
    type: 'render',
    render: () => h('div', { style: 'padding: 4px 12px 8px; font-size: 12px; color: #8b9bb4; white-space: nowrap; overflow: hidden; text-overflow: ellipsis' }, auth.me?.role === 'admin' ? t('role.admin') : t('role.viewer')),
  },
  { label: t('account.security'), key: 'security' },
  { type: 'divider', key: 'd1' },
  { label: t('account.logout'), key: 'logout' },
])

function onAccountSelect(key: string) {
  if (key === 'security') securityOpen.value = true
  else if (key === 'logout') onLogout()
}

async function onLogout() {
  dialog.warning({
    title: t('account.logout'),
    content: t('account.logout_confirm'),
    positiveText: t('account.logout_ok'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      await auth.logout()
      router.push('/login')
    },
  })
}
</script>

<template>
  <NLayout has-sider position="absolute">
    <NLayoutSider bordered collapse-mode="width" :collapsed-width="64" :width="212" :collapsed="collapsed" show-trigger @collapse="collapsed = true" @expand="collapsed = false">
      <div class="brand">
        <img class="logo" src="/logo.png" alt="" />
        <span v-if="!collapsed" class="title">{{ t('app.name') }} <em v-if="locale === 'zh'">GrayCrow</em></span>
      </div>
      <NMenu :value="activeKey" :options="items" :collapsed="collapsed" :collapsed-width="64" :collapsed-icon-size="20" @update:value="onSelect" />
    </NLayoutSider>

    <NLayout content-style="display: flex; flex-direction: column">
      <NLayoutHeader bordered class="header">
        <div>
          <div class="page-title">{{ t(String(route.meta.title)) }}</div>
        </div>
        <div class="row" style="gap: 14px">
          <NTooltip>
            <template #trigger>
              <button class="bell" type="button" :aria-label="t('nav.alerts')" @click="router.push('/alerts')">
                <NBadge dot processing color="#00e5ff" :show="pending > 0" :offset="[2, 0]">
                  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="#8b9bb4" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
                    <path d="M13.7 21a2 2 0 0 1-3.4 0" />
                  </svg>
                </NBadge>
              </button>
            </template>
            {{ t('alerts.pending_count', { n: pending }) }}
          </NTooltip>
          <NButton size="tiny" quaternary @click="setLocale(locale === 'zh' ? 'en' : 'zh')">
            {{ locale === 'zh' ? 'EN' : '中' }}
          </NButton>
          <NDropdown trigger="click" width="trigger" :options="accountOptions" @select="onAccountSelect">
            <div class="row" style="gap: 8px; cursor: pointer; min-width: 160px; max-width: 240px">
              <NAvatar round :size="26" style="background: #12324a; color: #00e5ff; flex: none">{{ (auth.me?.nickname || 'U').slice(0, 1) }}</NAvatar>
              <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap">{{ auth.me?.nickname }}</span>
            </div>
          </NDropdown>
        </div>
      </NLayoutHeader>
      <NLayoutContent style="flex: 1 1 0; min-height: 0; background: transparent">
        <RouterView />
      </NLayoutContent>
    </NLayout>
  </NLayout>
  <SecurityModal v-model:show="securityOpen" />
</template>

<style scoped>
.bell {
  display: inline-flex;
  align-items: center;
  padding: 2px 4px;
  border: 0;
  background: none;
  cursor: pointer;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 56px;
  padding: 0 18px;
  border-bottom: 1px solid rgba(0, 229, 255, 0.12);
}
.logo {
  width: 28px;
  height: 28px;
  flex: none;
  filter: drop-shadow(0 0 6px rgba(0, 229, 255, 0.35));
}
.title {
  font-weight: 600;
  letter-spacing: 1px;
}
.title em {
  font-style: normal;
  font-size: 11px;
  color: #8b9bb4;
  letter-spacing: 2px;
}
.header {
  flex: none;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 22px;
  backdrop-filter: blur(8px);
}
</style>
