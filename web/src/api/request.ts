import axios, { AxiosError } from 'axios'
import { createDiscreteApi } from 'naive-ui'
import { errorText, t } from '@/i18n'

const { message } = createDiscreteApi(['message'])

export const USE_MOCK = import.meta.env.VITE_USE_MOCK === '1'

export class ApiError extends Error {
  code: number

  constructor(code: number, msg: string) {
    super(msg)
    this.code = code
  }
}

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api/v1',
  withCredentials: true,
  timeout: 20000,
})

http.interceptors.request.use((cfg) => {
  if (cfg.method && cfg.method !== 'get') cfg.headers['X-Requested-With'] = 'graycrow'
  return cfg
})

http.interceptors.response.use(
  (res) => {
    const body = res.data
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code !== 0) throw new ApiError(body.code, errorText(body.code, body.message))
      return body.data
    }
    return body
  },
  (err: AxiosError<{ code?: number; message?: string }>) => {
    if (err.response?.status === 401) {
      message.warning(t('error.2001'))
      if (!location.pathname.startsWith('/login')) location.href = '/login'
      return Promise.reject(new ApiError(2001, t('error.2001')))
    }
    if (err.response?.status === 403) {
      message.error(t('error.3001'))
      return Promise.reject(new ApiError(3001, t('error.3001')))
    }
    message.error(errorText(err.response?.data?.code || 0, err.message || t('error.network')))
    return Promise.reject(err)
  },
)

async function req<T>(method: string, url: string, payload?: unknown): Promise<T> {
  if (USE_MOCK) {
    const { mockRequest } = await import('./mock')
    return mockRequest<T>(method, url, payload)
  }
  if (method === 'get') return http.get(url, { params: payload as object }) as unknown as Promise<T>
  return http.request({ method, url, data: payload }) as unknown as Promise<T>
}

export const get = <T>(url: string, params?: unknown) => req<T>('get', url, params)
export const post = <T>(url: string, data?: unknown) => req<T>('post', url, data)
export const put = <T>(url: string, data?: unknown) => req<T>('put', url, data)
export const patch = <T>(url: string, data?: unknown) => req<T>('patch', url, data)
export const del = <T>(url: string) => req<T>('delete', url)
