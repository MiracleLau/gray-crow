import type {
  Alert,
  AlertStatus,
  AllowEntry,
  ApiKey,
  AuditLog,
  Channel,
  Me,
  NotifyRecord,
  Overview,
  Page,
  QuotaPool,
  Rule,
  ScanRun,
  TrendPoint,
  User,
} from '../types'

const now = Date.now()
const iso = (msAgo: number) => new Date(now - msAgo).toISOString()

const repos = ['acme/backend', 'acme/web-console', 'devops-toolbox/deploy', 'thirdparty/sample-sdk', 'acme/mobile-app']
const keywords = ['acme.com', '@acme.com', 'acme-internal']
const detectors = ['aws-access-key', 'generic-password', 'github-pat', 'mysql-uri', '']
const statuses: AlertStatus[] = ['pending', 'pending', 'resolved', 'false_positive', 'ignored', 'allowlisted']

export const users: User[] = [
  { id: 1, username: 'admin', nickname: '系统管理员', role: 'admin', status: 1, totp_enabled: false, last_login_at: iso(60000), created_at: iso(86400000 * 30) },
  { id: 2, username: 'alice', nickname: 'Alice', role: 'viewer', status: 1, totp_enabled: true, last_login_at: iso(3600000), created_at: iso(86400000 * 12) },
  { id: 3, username: 'bob', nickname: 'Bob', role: 'viewer', status: 0, totp_enabled: false, created_at: iso(86400000 * 5) },
]

export const rules: Rule[] = [1, 2, 3].map((i) => ({
  id: i,
  name: ['acme 主站泄露巡检', '内部邮箱暴露监控', '核心仓库密钥巡检'][i - 1],
  description: '监控公开代码中是否出现站点凭据',
  keywords: i === 2 ? [keywords[1]] : [keywords[0], keywords[i]],
  query_template: '',
  schedule_type: 'interval',
  cron_expr: '',
  interval_sec: [3600, 21600, 1800][i - 1],
  severity: (['high', 'medium', 'high'] as const)[i - 1],
  enabled: i !== 3,
  dry_run: i === 2,
  next_scan_at: new Date(now + 60000 * i).toISOString(),
  last_scan_at: iso(600000 * i),
  alert_count: [42, 7, 13][i - 1],
  viewer_ids: i === 1 ? [2] : [2, 3],
  channel_ids: [1, 2],
  created_at: iso(86400000 * (20 - i)),
}))

export const alerts: Alert[] = Array.from({ length: 57 }, (_, i) => {
  const st = statuses[i % statuses.length]
  return {
    id: 57 - i,
    rule_id: rules[i % rules.length].id,
    rule_name: rules[i % rules.length].name,
    repo_full_name: repos[i % repos.length],
    repo_url: `https://github.com/${repos[i % repos.length]}`,
    html_url: `https://github.com/${repos[i % repos.length]}/blob/main/config/app-${i}.yaml`,
    path: `config/app-${i}.yaml`,
    matched_keyword: keywords[i % keywords.length],
    detector_id: detectors[i % detectors.length],
    snippet: `db:\n  host: db.acme.com\n  username: acme\n  password: "S3cr3t-Pa55w0rd-${i}"\napi_key: AKIAIOSFODNN7EXAMPLE`,
    status: st,
    first_seen_at: iso(3600000 * (i + 1)),
    last_seen_at: iso(600000 * i),
    handled_by_name: st === 'pending' ? undefined : '系统管理员',
    handled_at: st === 'pending' ? undefined : iso(600000 * i),
    note: st === 'false_positive' ? '示例域名，非真实凭据' : '',
  }
})

export const apiKeys: ApiKey[] = [
  { id: 1, name: '运维号-1', token_hint: '****a1b2', account_login: 'ops-a', scopes: 'repo', enabled: true, sort_order: 1, status: 'active', quota_limit: 10, quota_remaining: 8, quota_reset_at: new Date(now + 42000).toISOString(), quota_updated_at: iso(3000), quota_source: 'header', fail_count: 0, last_error: '', total_used: 1284, last_used_at: iso(12000) },
  { id: 2, name: '运维号-2', token_hint: '****c3d4', account_login: 'ops-a', scopes: 'repo', enabled: true, sort_order: 2, status: 'low', quota_limit: 10, quota_remaining: 1, quota_reset_at: new Date(now + 42000).toISOString(), quota_updated_at: iso(3000), quota_source: 'endpoint', fail_count: 0, last_error: '', total_used: 980, last_used_at: iso(45000) },
  { id: 3, name: '安全组号', token_hint: '****e5f6', account_login: 'sec-team', scopes: 'repo,read:org', enabled: true, sort_order: 3, status: 'active', quota_limit: 10, quota_remaining: 9, quota_reset_at: new Date(now + 42000).toISOString(), quota_updated_at: iso(3000), quota_source: 'header', fail_count: 0, last_error: '', total_used: 236, last_used_at: iso(9000) },
  { id: 4, name: '备用号', token_hint: '****g7h8', account_login: 'ops-b', scopes: 'repo', enabled: false, sort_order: 4, status: 'disabled', quota_limit: 10, quota_remaining: 10, quota_updated_at: iso(7200000), quota_source: 'endpoint', fail_count: 0, last_error: '', total_used: 12 },
]

export const channels: Channel[] = [
  { id: 1, name: '安全告警群', type: 'feishu', config: { webhook: 'https://open.feishu.cn/open-apis/bot/v2/hook/***' }, enabled: true, last_status: 'success', last_error: '', last_sent_at: iso(120000) },
  { id: 2, name: '运维企微群', type: 'wecom', config: { webhook: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=***' }, enabled: true, last_status: 'success', last_error: '', last_sent_at: iso(3600000) },
  { id: 3, name: '钉钉值班群', type: 'dingtalk', config: { webhook: 'https://oapi.dingtalk.com/robot/send?access_token=***', at_mobiles: ['13800000000'] }, enabled: true, last_status: 'failed', last_error: 'HTTP 410: robot not exist', last_sent_at: iso(7200000) },
  { id: 4, name: '自建网关', type: 'webhook', config: { url: 'https://gateway.internal/hooks/graycrow', method: 'POST', body_template: '{"text":"{{title}}"}' }, enabled: true, last_status: 'success', last_error: '', last_sent_at: iso(600000) },
  { id: 5, name: '邮件通知', type: 'smtp', config: { host: 'smtp.internal', port: 587, from: 'graycrow@acme.com', to: ['sec@acme.com'] }, enabled: false, last_status: '', last_error: '' },
]

export const allowlist: AllowEntry[] = [
  { id: 1, scope: 'global', repo_full_name: 'thirdparty/sample-sdk', reason: '示例文档仓库，含测试占位凭据', created_by_name: '系统管理员', created_at: iso(86400000 * 3) },
  { id: 2, scope: 'rule', rule_id: 1, rule_name: rules[0].name, repo_full_name: 'devops-toolbox/deploy', reason: '内部工具误报', expires_at: new Date(now + 86400000 * 30).toISOString(), created_by_name: '系统管理员', created_at: iso(86400000) },
]

export const scanRuns: ScanRun[] = Array.from({ length: 20 }, (_, i) => ({
  id: 20 - i,
  rule_id: rules[i % rules.length].id,
  key_name: apiKeys[i % 3].name,
  query: `"acme.com" password -is:fork`,
  status: 'success',
  result_count: 100 * (i % 10),
  new_alert_count: i % 5,
  quota_remaining_after: 10 - (i % 10),
  error: '',
  started_at: iso(300000 * i),
  finished_at: iso(300000 * i - 4000),
}))

export const notifyRecords: NotifyRecord[] = Array.from({ length: 8 }, (_, i) => ({
  id: 8 - i,
  rule_id: 1,
  rule_name: 'acme 泄露巡检',
  channel_name: channels[i % 4].name,
  alert_count: 1 + (i % 3) * 4,
  status: i % 4 === 3 ? 'failed' : 'success',
  http_status: i % 4 === 3 ? 410 : 200,
  attempt: 1 + (i % 3),
  error: i % 4 === 3 ? 'HTTP 410: robot not exist' : '',
  content: '【灰鸦告警汇总】acme 泄露巡检：本次扫描发现 5 条新问题',
  sent_at: iso(600000 * i),
}))

export const auditLogs: AuditLog[] = Array.from({ length: 30 }, (_, i) => ({
  id: 30 - i,
  actor_name: i % 3 === 0 ? '系统管理员' : 'Alice',
  action: ['login', 'alert.status', 'rule.update', 'apikey.check', 'allowlist.create'][i % 5],
  target: ['-', `alert#${57 - i}`, 'rule#1', 'key#1', 'thirdparty/sample-sdk'][i % 5],
  detail: '',
  ip: '10.0.0.' + (10 + (i % 20)),
  created_at: iso(600000 * i),
}))

export const trend: TrendPoint[] = Array.from({ length: 30 }, (_, i) => ({
  date: new Date(now - 86400000 * (29 - i)).toISOString().slice(0, 10),
  new: 3 + ((i * 7) % 12),
  resolved: 2 + ((i * 5) % 9),
  false_positive: 1 + ((i * 3) % 5),
}))

export const overview: Overview = {
  today_new: 12,
  pending: alerts.filter((a) => a.status === 'pending').length,
  false_positive_rate: 0.18,
  resolved_rate: 0.52,
  avg_handle_minutes: 74,
  pool_remaining: apiKeys.filter((k) => k.enabled).reduce((s, k) => s + k.quota_remaining, 0),
  pool_limit: 30,
  pool_accounts: 2,
}

function paginate<T>(items: T[], query: URLSearchParams): Page<T> {
  const page = Number(query.get('page') || 1)
  const size = Number(query.get('page_size') || 20)
  return { items: items.slice((page - 1) * size, page * size), total: items.length, page, page_size: size }
}

function parse(url: string) {
  const [path, search = ''] = url.split('?')
  return { path, query: new URLSearchParams(search) }
}

const me: Me = { ...users[0], rule_ids: [1, 2, 3] }

const AUTH_KEY = 'gc-mock-user'

function signedIn() {
  return sessionStorage.getItem(AUTH_KEY)
}

type Handler = (m: RegExpMatchArray, query: URLSearchParams, body: unknown) => unknown

const table: [string, RegExp, Handler][] = [
  ['post', /^\/auth\/login$/, (_m, _q, body) => {
    const b = body as { username: string; password: string }
    if (!b?.username || !b?.password) throw new Error('用户名或密码错误')
    const role = b.username === 'admin' ? 'admin' : 'viewer'
    sessionStorage.setItem(AUTH_KEY, b.username)
    return { ...me, username: b.username, nickname: b.username, role }
  }],
  ['post', /^\/auth\/logout$/, () => {
    sessionStorage.removeItem(AUTH_KEY)
    return {}
  }],
  ['post', /^\/auth\/totp\/setup$/, () => ({ secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP', otpauth_url: 'otpauth://totp/GrayCrow:admin?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=GrayCrow' })],
  ['post', /^\/auth\/totp\/enable$/, () => ({ recovery_codes: ['A1B2C3D4', 'E5F6G7H8', 'J9K0L1M2', 'N3P4Q5R6', 'S7T8U9V0', 'W1X2Y3Z4', 'A5B6C7D8', 'E9F0G1H2', 'J3K4L5M6', 'N7P8Q9R0'] })],
  ['post', /^\/auth\/totp\/disable$/, () => ({})],
  ['get', /^\/auth\/me$/, () => {
    const username = signedIn()
    if (!username) throw new Error('未登录')
    return { ...me, username, nickname: username === 'admin' ? '系统管理员' : username, role: username === 'admin' ? 'admin' : 'viewer' }
  }],
  ['get', /^\/rules$/, (_m, q) => paginate(rules, q)],
  ['get', /^\/schedule\/preview$/, (_m, q) => {
    const expr = (q.get('expr') || '').trim()
    if (expr.split(/\s+/).length !== 5) throw new Error('cron 表达式需要 5 段（分 时 日 月 周）')
    const base = Math.ceil(now / 3600000) * 3600000
    return {
      timezone: 'Asia/Shanghai',
      next: [1, 2, 3].map((i) => new Date(base + i * 3600000).toISOString().slice(0, 16).replace('T', ' ')),
    }
  }],
  ['get', /^\/rules\/(\d+)\/runs$/, () => paginate(scanRuns, new URLSearchParams())],
  ['get', /^\/alerts$/, (_m, q) => {
    let list = alerts
    const status = q.get('status')
    const ruleId = q.get('rule_id')
    const repo = q.get('repo')
    if (status) list = list.filter((a) => a.status === status)
    if (ruleId) list = list.filter((a) => a.rule_id === Number(ruleId))
    if (repo) list = list.filter((a) => a.repo_full_name.includes(repo))
    return paginate(list, q)
  }],
  ['get', /^\/alerts\/(\d+)$/, (m) => {
    const id = Number(m[1])
    const found = alerts.find((a) => a.id === id) || alerts[0]
    return {
      ...found,
      events: [
        { id: 1, actor_name: 'system', action: 'create', to_status: 'pending', note: '扫描命中，二次判定为 generic-credential', created_at: found.first_seen_at },
        { id: 2, actor_name: 'system', action: 'notify', note: '已投递至 安全告警群', created_at: found.first_seen_at },
      ],
    }
  }],
  ['patch', /^\/alerts\/(\d+)\/status$/, (m, _q, body) => {
    const { status, note } = body as { status: AlertStatus; note: string }
    const found = alerts.find((a) => a.id === Number(m[1]))
    if (found) {
      found.status = status
      found.note = note || found.note
      found.handled_by_name = '系统管理员'
      found.handled_at = new Date().toISOString()
    }
    return found || {}
  }],
  ['post', /^\/alerts\/bulk-status$/, (_m, _q, body) => {
    const { ids, status } = body as { ids: number[]; status: AlertStatus }
    ids.forEach((id) => {
      const found = alerts.find((a) => a.id === id)
      if (found) found.status = status
    })
    return { updated: ids, skipped: [] }
  }],
  ['post', /^\/alerts\/(\d+)\/allowlist$/, (m, _q, body) => {
    const { scope, reason } = body as { scope: 'global' | 'rule'; reason: string }
    const found = alerts.find((a) => a.id === Number(m[1]))
    if (found) {
      found.status = 'allowlisted'
      allowlist.unshift({ id: allowlist.length + 1, scope, rule_id: scope === 'rule' ? found.rule_id : undefined, rule_name: found.rule_name, repo_full_name: found.repo_full_name, reason, created_by_name: '系统管理员', created_at: new Date().toISOString() })
    }
    return found || {}
  }],
  ['post', /^\/alerts\/(\d+)\/renotify$/, () => ({ ok: true })],
  ['get', /^\/notify-records$/, (_m, q) => paginate(notifyRecords, q)],
  ['get', /^\/allowlist$/, (_m, q) => paginate(allowlist, q)],
  ['delete', /^\/allowlist\/(\d+)$/, (m) => {
    const idx = allowlist.findIndex((a) => a.id === Number(m[1]))
    if (idx >= 0) allowlist.splice(idx, 1)
    return {}
  }],
  ['get', /^\/api-keys$/, () => apiKeys],
  ['get', /^\/api-keys\/quota$/, (): QuotaPool => ({
    updated_at: new Date().toISOString(),
    pool: {
      limit: apiKeys.filter((k) => k.enabled).reduce((s, k) => s + k.quota_limit, 0),
      remaining: apiKeys.filter((k) => k.enabled).reduce((s, k) => s + k.quota_remaining, 0),
      accounts: new Set(apiKeys.filter((k) => k.enabled).map((k) => k.account_login)).size,
      per_minute: apiKeys.filter((k) => k.enabled).reduce((s, k) => s + k.quota_remaining, 0),
    },
    keys: apiKeys,
  })],
  ['post', /^\/api-keys\/(\d+)\/check$/, (m) => apiKeys.find((k) => k.id === Number(m[1])) || {}],
  ['post', /^\/api-keys\/check-all$/, () => ({ checked: apiKeys.length })],
  ['post', /^\/api-keys\/reorder$/, () => ({ ok: true })],
  ['get', /^\/api-keys\/usage$/, () => apiKeys.map((k) => ({ id: k.id, name: k.name, account_login: k.account_login, total_used: k.total_used, alerts: (k.total_used / 20) | 0 }))],
  ['get', /^\/channels$/, () => channels],
  ['post', /^\/channels\/(\d+)\/test$/, () => ({ ok: true, response: '{"code":0,"msg":"success"}' })],
  ['get', /^\/users$/, (_m, q) => paginate(users, q)],
  ['get', /^\/audit-logs$/, (_m, q) => paginate(auditLogs, q)],
  ['get', /^\/settings$/, () => ({ default_interval_sec: 3600, dry_run_days: 3 })],
  ['get', /^\/stats\/overview$/, () => overview],
  ['get', /^\/stats\/trend$/, () => trend],
  ['get', /^\/stats\/by-rule$/, () => rules.map((r) => ({ rule_id: r.id, rule_name: r.name, total: r.alert_count, pending: (r.alert_count / 3) | 0 }))],
  ['get', /^\/stats\/by-status$/, () => statuses.slice(0, 5).map((s) => ({ status: s, total: alerts.filter((a) => a.status === s).length }))],
  ['get', /^\/stats\/quota$/, () => trend.map((t) => ({ date: t.date, remaining: 6 + (t.new % 5), used: t.new }))],
]

export async function mockRequest<T>(method: string, url: string, payload?: unknown): Promise<T> {
  await new Promise((r) => setTimeout(r, 120))
  // 真接口走 axios 的 params，这里手动拼到 query，保证 mock 也能读到过滤条件
  if (method === 'get' && payload && typeof payload === 'object') {
    const qs = new URLSearchParams(
      Object.entries(payload as Record<string, unknown>)
        .filter(([, v]) => v !== undefined && v !== null)
        .map(([k, v]) => [k, String(v)]),
    ).toString()
    if (qs) url += (url.includes('?') ? '&' : '?') + qs
  }
  const { path, query } = parse(url)
  const body = method === 'get' ? undefined : payload
  for (const [m, re, handler] of table) {
    if (m !== method) continue
    const match = path.match(re)
    if (match) return handler(match, query, body) as T
  }
  if (method === 'put' || method === 'post' || method === 'delete' || method === 'patch') return {} as T
  throw new Error(`mock 未实现: ${method.toUpperCase()} ${path}`)
}
