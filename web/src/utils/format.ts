import { locale, t } from '@/i18n'

export function relTime(input?: string | number | null): string {
  if (!input) return t('common.none')
  const ms = new Date(input).getTime()
  const diff = Date.now() - ms
  const abs = Math.abs(diff)
  const units: [number, string][] = [
    [86400000, 'unit.day'],
    [3600000, 'unit.hour'],
    [60000, 'unit.minute'],
    [1000, 'unit.second'],
  ]
  for (const [unit, key] of units) {
    if (abs < unit) continue
    const value = Math.floor(abs / unit)
    return diff >= 0
      ? t('time.ago', { v: value, unit: t(key) })
      : t('time.later', { v: value, unit: t(key) })
  }
  return t('time.just_now')
}

export function dateTime(input?: string | number | null): string {
  if (!input) return t('common.none')
  return new Date(input).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', { hour12: false })
}

export function countdown(input?: string | number | null, nowMs = Date.now()): string {
  if (!input) return t('common.none')
  const diff = new Date(input).getTime() - nowMs
  if (diff <= 0) return t('time.reset')
  const s = Math.floor(diff / 1000)
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

export function num(n?: number | null): string {
  return (n ?? 0).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
}
