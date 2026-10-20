import { computed, ref } from 'vue'
import { zh } from './zh'
import { en } from './en'

export type Locale = 'zh' | 'en'

const dicts: Record<Locale, Record<string, string>> = { zh, en }
const stored = localStorage.getItem('gc-locale')
export const locale = ref<Locale>(stored === 'en' ? 'en' : 'zh')

export function setLocale(next: Locale) {
  locale.value = next
  localStorage.setItem('gc-locale', next)
  document.documentElement.lang = next === 'zh' ? 'zh-CN' : 'en'
}

/** 缺 key 时回退中文、再回退 key 本身：漏翻译一眼能看出，不会渲染成空白。 */
export function t(key: string, params?: Record<string, string | number>): string {
  const raw = dicts[locale.value][key] ?? dicts.zh[key] ?? key
  if (!params) return raw
  return raw.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? `{${name}}`))
}

export const isZh = computed(() => locale.value === 'zh')

/** 后端错误码文案：命中用本地化文案，未命中沿用服务端 message。 */
export function errorText(code: number, fallback?: string): string {
  const key = `error.${code}`
  const hit = dicts[locale.value][key]
  return hit || fallback || t('error.unknown')
}
