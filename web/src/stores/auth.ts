import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { authApi } from '@/api/endpoints'
import { isTotpChallenge } from '@/api/types'
import type { Me } from '@/api/types'

export const useAuthStore = defineStore('auth', () => {
  const me = ref<Me | null>(null)
  const loaded = ref(false)
  const isAdmin = computed(() => me.value?.role === 'admin')
  const pendingTicket = ref('')

  async function login(username: string, password: string) {
    const res = await authApi.login(username, password)
    if (isTotpChallenge(res)) {
      pendingTicket.value = res.ticket
      return true
    }
    me.value = res
    loaded.value = true
    return false
  }

  async function verifyTotp(code: string, recovery = false) {
    me.value = await authApi.totp({ ticket: pendingTicket.value, code, recovery })
    pendingTicket.value = ''
    loaded.value = true
  }

  function cancelTotp() {
    pendingTicket.value = ''
  }

  async function logout() {
    try {
      await authApi.logout()
    } finally {
      me.value = null
      pendingTicket.value = ''
    }
  }

  async function ensure() {
    if (loaded.value) return me.value
    try {
      me.value = await authApi.me()
    } catch {
      me.value = null
    }
    loaded.value = true
    return me.value
  }

  return { me, loaded, isAdmin, pendingTicket, login, verifyTotp, cancelTotp, logout, ensure }
})
