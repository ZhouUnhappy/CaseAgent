import axios from 'axios'
import { notifyApiError } from '../utils/error'

const client = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api/v1',
  timeout: 30000,
})

client.interceptors.response.use(
  (response) => response,
  (error) => {
    const normalized = normalizeError(error)
    notifyApiError(normalized)
    return Promise.reject(normalized)
  },
)

function normalizeError(error) {
  if (error.response) {
    const { status, data, config } = error.response
    const message = (data && (data.error || data.message)) || error.message || `HTTP ${status}`
    return {
      name: 'ApiError',
      status,
      url: config?.url,
      method: config?.method,
      message,
      raw: data,
      retryable: status >= 500 || status === 408 || status === 429,
    }
  }
  if (error.request) {
    return {
      name: 'NetworkError',
      status: 0,
      url: error.config?.url,
      method: error.config?.method,
      message: error.message || '网络异常，请稍后重试',
      retryable: true,
    }
  }
  return {
    name: 'ClientError',
    status: -1,
    message: error.message || '未知错误',
    retryable: false,
  }
}

export default client
