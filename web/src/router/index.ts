import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import BasicLayout from '@/layouts/BasicLayout.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { public: true, title: 'nav.login' } },
    {
      path: '/',
      component: BasicLayout,
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: 'nav.dashboard' } },
        { path: 'alerts', name: 'alerts', component: () => import('@/views/AlertsView.vue'), meta: { title: 'nav.alerts' } },
        { path: 'alerts/:id', name: 'alert-detail', component: () => import('@/views/AlertDetailView.vue'), meta: { title: 'nav.alert_detail', activeMenu: '/alerts' } },
        { path: 'notify-records', name: 'notify-records', component: () => import('@/views/NotifyRecordsView.vue'), meta: { title: 'nav.notify' } },
        { path: 'stats', name: 'stats', component: () => import('@/views/StatsView.vue'), meta: { title: 'nav.stats' } },
        { path: 'rules', name: 'rules', component: () => import('@/views/RulesView.vue'), meta: { title: 'nav.rules', admin: true } },
        { path: 'allowlist', name: 'allowlist', component: () => import('@/views/AllowlistView.vue'), meta: { title: 'nav.allowlist', admin: true } },
        { path: 'api-keys', name: 'api-keys', component: () => import('@/views/ApiKeysView.vue'), meta: { title: 'nav.api_keys', admin: true } },
        { path: 'channels', name: 'channels', component: () => import('@/views/ChannelsView.vue'), meta: { title: 'nav.channels', admin: true } },
        { path: 'templates', name: 'templates', component: () => import('@/views/TemplatesView.vue'), meta: { title: 'nav.templates', admin: true } },
        { path: 'users', name: 'users', component: () => import('@/views/UsersView.vue'), meta: { title: 'nav.users', admin: true } },
        { path: 'audit-logs', name: 'audit-logs', component: () => import('@/views/AuditLogsView.vue'), meta: { title: 'nav.audit', admin: true } },
        { path: 'settings', name: 'settings', component: () => import('@/views/SettingsView.vue'), meta: { title: 'nav.settings', admin: true } },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  await auth.ensure()
  if (!to.meta.public && !auth.me) return { name: 'login', query: { redirect: to.fullPath } }
  if (to.name === 'login' && auth.me) return { path: '/dashboard' }
  if (to.meta.admin && !auth.isAdmin) return { path: '/dashboard' }
  return true
})

export default router
