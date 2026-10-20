export type Role = 'admin' | 'viewer'
export type AlertStatus = 'pending' | 'resolved' | 'false_positive' | 'ignored' | 'allowlisted'
export type Severity = 'high' | 'medium' | 'low'
export type KeyStatus = 'active' | 'low' | 'cooling' | 'invalid' | 'disabled'
export type ChannelType = 'webhook' | 'smtp' | 'feishu' | 'wecom' | 'dingtalk'

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface User {
  id: number
  username: string
  nickname: string
  role: Role
  status: number
  totp_enabled: boolean
  last_login_at?: string
  created_at: string
}

export interface Me extends User {
  rule_ids: number[]
}

export interface TotpChallenge {
  totp_required: true
  ticket: string
}

export type LoginResult = Me | TotpChallenge

export const isTotpChallenge = (r: LoginResult): r is TotpChallenge => 'totp_required' in r

export interface DiagnosisSuggestion {
  kind: 'drop_keyword' | 'keep_dry_run' | 'ready' | 'ok' | 'no_data'
  keyword?: string
  value?: number
}

export interface RuleDiagnosis {
  window_days: number
  dry_run: boolean
  total: number
  judged: number
  false_positive: number
  false_positive_rate: number
  detected: number
  context_only: number
  by_keyword: { keyword: string; total: number; context_only: number; false_positive: number }[]
  by_detector: { detector: string; total: number }[]
  suggestions: DiagnosisSuggestion[]
}

export interface ApiKey {
  id: number
  name: string
  token_hint: string
  account_login: string
  scopes: string
  enabled: boolean
  sort_order: number
  status: KeyStatus
  quota_limit: number
  quota_remaining: number
  quota_reset_at?: string
  quota_updated_at?: string
  quota_source: 'header' | 'endpoint'
  cooldown_until?: string
  fail_count: number
  last_error: string
  total_used: number
  last_used_at?: string
}

export interface QuotaPool {
  updated_at: string
  pool: { limit: number; remaining: number; accounts: number; per_minute: number }
  keys: ApiKey[]
}

export interface Rule {
  id: number
  name: string
  description: string
  keywords: string[]
  query_template: string
  schedule_type: 'interval' | 'cron'
  cron_expr: string
  interval_sec: number
  severity: Severity
  enabled: boolean
  dry_run: boolean
  next_scan_at?: string
  last_scan_at?: string
  alert_count: number
  viewer_ids: number[]
  channel_ids: number[]
  created_at: string
}

export interface AlertEvent {
  id: number
  actor_name: string
  action: string
  from_status?: AlertStatus
  to_status?: AlertStatus
  note: string
  created_at: string
}

export interface Alert {
  id: number
  rule_id: number
  rule_name: string
  repo_full_name: string
  repo_url: string
  html_url: string
  path: string
  matched_keyword: string
  detector_id: string
  snippet: string
  status: AlertStatus
  first_seen_at: string
  last_seen_at: string
  handled_by_name?: string
  handled_at?: string
  note: string
  events?: AlertEvent[]
}

export interface AllowEntry {
  id: number
  scope: 'global' | 'rule'
  rule_id?: number
  rule_name?: string
  repo_full_name: string
  reason: string
  expires_at?: string
  created_by_name: string
  created_at: string
}

export interface Channel {
  id: number
  name: string
  type: ChannelType
  template_id?: number
  config: Record<string, unknown>
  enabled: boolean
  last_status: string
  last_error: string
  last_sent_at?: string
}

export interface NotifyTemplate {
  id: number
  name: string
  title: string
  body: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface ScanRun {
  id: number
  rule_id: number
  key_name: string
  query: string
  status: 'success' | 'failed' | 'partial'
  result_count: number
  new_alert_count: number
  quota_remaining_after: number
  error: string
  started_at: string
  finished_at?: string
}

export interface NotifyRecord {
  id: number
  rule_id: number
  rule_name: string
  alert_id?: number
  channel_name: string
  alert_count: number
  status: 'success' | 'failed'
  http_status?: number
  attempt: number
  error: string
  content: string
  sent_at: string
}

export interface AuditLog {
  id: number
  actor_name: string
  action: string
  target: string
  detail: string
  ip: string
  created_at: string
}

export interface Overview {
  today_new: number
  pending: number
  false_positive_rate: number
  resolved_rate: number
  avg_handle_minutes: number
  pool_remaining: number
  pool_limit: number
  pool_accounts: number
}

export interface TrendPoint {
  date: string
  new: number
  resolved: number
  false_positive: number
}

export interface KeyUsage {
  id: number
  name: string
  account_login: string
  total_used: number
  alerts: number
}
