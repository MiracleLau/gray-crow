import { del, get, patch, post, put } from './request'
import type {
  Alert,
  AlertStatus,
  AllowEntry,
  ApiKey,
  AuditLog,
  Channel,
  KeyUsage,
  LoginResult,
  Me,
  NotifyRecord,
  NotifyTemplate,
  Overview,
  Page,
  QuotaPool,
  Rule,
  RuleDiagnosis,
  ScanRun,
  TrendPoint,
  User,
} from './types'

export const authApi = {
  login: (username: string, password: string) => post<LoginResult>('/auth/login', { username, password }),
  totp: (data: { ticket: string; code: string; recovery?: boolean }) => post<Me>('/auth/totp', data),
  logout: () => post<unknown>('/auth/logout'),
  me: () => get<Me>('/auth/me'),
  changePassword: (data: { old_password: string; new_password: string }) => post<unknown>('/auth/password', data),
  setupTotp: () => post<{ secret: string; otpauth_url: string }>('/auth/totp/setup'),
  enableTotp: (code: string) => post<{ recovery_codes: string[] }>('/auth/totp/enable', { code }),
  disableTotp: (password: string) => post<unknown>('/auth/totp/disable', { password }),
}

export const ruleApi = {
  list: (params?: object) => get<Page<Rule>>('/rules', params),
  detail: (id: number) => get<Rule>(`/rules/${id}`),
  create: (data: Partial<Rule> & { run_now?: boolean }) => post<Rule>('/rules', data),
  update: (id: number, data: Partial<Rule>) => put<Rule>(`/rules/${id}`, data),
  remove: (id: number) => del<unknown>(`/rules/${id}`),
  scan: (id: number) => post<unknown>(`/rules/${id}/scan`),
  runs: (id: number, params?: object) => get<Page<ScanRun>>(`/rules/${id}/runs`, params),
  diagnose: (id: number) => get<RuleDiagnosis>(`/rules/${id}/diagnose`),
  previewSchedule: (expr: string) => get<{ timezone: string; next: string[] }>('/schedule/preview', { expr }),
  setViewers: (id: number, viewerIds: number[]) => put<unknown>(`/rules/${id}/viewers`, { viewer_ids: viewerIds }),
  setChannels: (id: number, channelIds: number[]) => put<unknown>(`/rules/${id}/channels`, { channel_ids: channelIds }),
}

export const alertApi = {
  list: (params?: object) => get<Page<Alert>>('/alerts', params),
  detail: (id: number) => get<Alert>(`/alerts/${id}`),
  setStatus: (id: number, status: AlertStatus, note = '') => patch<Alert>(`/alerts/${id}/status`, { status, note }),
  bulkStatus: (ids: number[], status: AlertStatus, note = '') => post<{ updated: number[]; skipped: unknown[] }>('/alerts/bulk-status', { ids, status, note }),
  allowlist: (id: number, scope: 'global' | 'rule', reason: string, expiresAt?: string) => post<Alert>(`/alerts/${id}/allowlist`, { scope, reason, expires_at: expiresAt }),
  renotify: (id: number) => post<unknown>(`/alerts/${id}/renotify`),
  remove: (id: number) => del<{ deleted: number }>(`/alerts/${id}`),
  bulkDelete: (ids: number[]) => post<{ deleted: number }>('/alerts/bulk-delete', { ids }),
  exportUrl: (params: object) => `/api/v1/stats/export?type=alerts&${new URLSearchParams(params as Record<string, string>).toString()}`,
}

export const notifyApi = {
  list: (params?: object) => get<Page<NotifyRecord>>('/notify-records', params),
  remove: (id: number) => del<{ deleted: number }>(`/notify-records/${id}`),
  bulkDelete: (ids: number[]) => post<{ deleted: number }>('/notify-records/bulk-delete', { ids }),
}

export const allowApi = {
  list: (params?: object) => get<Page<AllowEntry>>('/allowlist', params),
  create: (data: Partial<AllowEntry> & { scope: 'global' | 'rule'; repo_full_name: string }) => post<AllowEntry>('/allowlist', data),
  remove: (id: number) => del<unknown>(`/allowlist/${id}`),
}

export const keyApi = {
  list: () => get<ApiKey[]>('/api-keys'),
  create: (data: { name: string; token: string }) => post<ApiKey>('/api-keys', data),
  update: (id: number, data: Partial<ApiKey> & { token?: string }) => put<ApiKey>(`/api-keys/${id}`, data),
  remove: (id: number) => del<unknown>(`/api-keys/${id}`),
  quota: () => get<QuotaPool>('/api-keys/quota'),
  check: (id: number) => post<ApiKey>(`/api-keys/${id}/check`),
  checkAll: () => post<{ checked: number }>('/api-keys/check-all'),
  reorder: (ids: number[]) => post<unknown>('/api-keys/reorder', { ids }),
  usage: (range = '7d') => get<KeyUsage[]>('/api-keys/usage', { range }),
}

export const channelApi = {
  list: () => get<Channel[]>('/channels'),
  create: (data: Partial<Channel> & { secret?: string; headers?: string }) => post<Channel>('/channels', data),
  update: (id: number, data: Partial<Channel> & { secret?: string; headers?: string }) => put<Channel>(`/channels/${id}`, data),
  remove: (id: number) => del<unknown>(`/channels/${id}`),
  test: (id: number) => post<{ ok: boolean; response: string }>(`/channels/${id}/test`),
  testDraft: (data: Partial<Channel> & { secret?: string; headers?: string }) => post<{ ok: boolean; response: string }>('/channels/test', data),
}

export const templateApi = {
  list: () => get<NotifyTemplate[]>('/templates'),
  create: (data: Partial<NotifyTemplate>) => post<NotifyTemplate>('/templates', data),
  update: (id: number, data: Partial<NotifyTemplate>) => put<NotifyTemplate>(`/templates/${id}`, data),
  remove: (id: number) => del<unknown>(`/templates/${id}`),
}

export const userApi = {
  list: (params?: object) => get<Page<User>>('/users', params),
  create: (data: Partial<User> & { password: string }) => post<User>('/users', data),
  update: (id: number, data: Partial<User>) => put<User>(`/users/${id}`, data),
  remove: (id: number) => del<unknown>(`/users/${id}`),
  resetPassword: (id: number, password: string) => patch<unknown>(`/users/${id}/password`, { password }),
  setStatus: (id: number, status: number) => patch<unknown>(`/users/${id}/status`, { status }),
  resetTotp: (id: number) => del<unknown>(`/users/${id}/totp`),
}

export const statsApi = {
  overview: () => get<Overview>('/stats/overview'),
  trend: (range = '30d') => get<TrendPoint[]>('/stats/trend', { range }),
  byRule: () => get<{ rule_id: number; rule_name: string; total: number; pending: number }[]>('/stats/by-rule'),
  byStatus: () => get<{ status: AlertStatus; total: number }[]>('/stats/by-status'),
  quota: (range = '7d') => get<{ date: string; remaining: number; used: number }[]>('/stats/quota', { range }),
}

export const auditApi = {
  list: (params?: object) => get<Page<AuditLog>>('/audit-logs', params),
}

export const settingsApi = {
  get: () => get<Record<string, unknown>>('/settings'),
  update: (data: Record<string, unknown>) => put<unknown>('/settings', data),
}
