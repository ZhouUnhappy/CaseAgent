import axios from 'axios'
import { notifyApiError } from '../utils/error'

const TENANT_STORAGE_KEY = 'caseagent.tenant_slug'
const OPERATOR_ID_STORAGE_KEY = 'caseagent.operator_id'
const OPERATOR_NAME_STORAGE_KEY = 'caseagent.operator_name'

const client = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api/v1',
  timeout: 30000,
})

// Inject X-Tenant-ID from localStorage on every request. /tenants endpoints
// don't need it but accept it; sending unconditionally keeps the rule simple.
client.interceptors.request.use((config) => {
  const slug = localStorage.getItem(TENANT_STORAGE_KEY)
  if (slug && !config.headers['X-Tenant-ID']) {
    config.headers['X-Tenant-ID'] = slug
  }
  const operatorId = localStorage.getItem(OPERATOR_ID_STORAGE_KEY)
  const operatorName = localStorage.getItem(OPERATOR_NAME_STORAGE_KEY)
  if (operatorId && !config.headers['X-Operator-ID']) {
    config.headers['X-Operator-ID'] = operatorId
  }
  if (operatorName && !config.headers['X-Operator-Name']) {
    config.headers['X-Operator-Name'] = operatorName
  }
  return config
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
