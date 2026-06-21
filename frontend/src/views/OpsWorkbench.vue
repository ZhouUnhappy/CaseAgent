<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import {
  Close,
  CopyDocument,
  Refresh,
  RefreshRight,
  View,
} from '@element-plus/icons-vue'
import { cancelJob, listJobs, replayJob, retryJob } from '../api/jobs'
import { getFeedbackSummary, getOpsMetrics, getOpsPreflight, getQualityOverview } from '../api/ops'
import { listWorkflows } from '../api/workflows'
import { useTenantStore } from '../stores/tenant'
import { formatDateTime as formatDate } from '../utils/date'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel } from '../utils/jobs'

const router = useRouter()
const tenantStore = useTenantStore()
const { activeItems: tenants, currentSlug, loading: tenantLoading } = storeToRefs(tenantStore)
const OPERATOR_ID_STORAGE_KEY = 'caseagent.operator_id'
const OPERATOR_NAME_STORAGE_KEY = 'caseagent.operator_name'

const activeTab = ref('overview')
const lastRefreshedAt = ref('')
const jobs = ref([])
const workflows = ref([])
const metrics = ref(null)
const preflight = ref(null)
const feedbackSummary = ref(null)
const qualityOverview = ref(null)
const jobsLoading = ref(false)
const workflowsLoading = ref(false)
const metricsLoading = ref(false)
const preflightLoading = ref(false)
const preflightCopying = ref(false)
const feedbackLoading = ref(false)
const qualityLoading = ref(false)
const actingJobId = ref(0)
const metricsRange = ref([])
const qualityRange = ref([])
const advancedQualitySections = ref([])
const tabLoaded = reactive({
  overview: false,
  runs: false,
  quality: false,
  models: false,
  environment: false,
})

const jobFilters = reactive({
  job_type: '',
  status: '',
  resource_type: '',
  resource_id: '',
})
const metricsFilters = reactive({
  provider: '',
  model: '',
  workflow_type: '',
  task_id: '',
})
const qualityFilters = reactive({
  task_id: '',
  feedback_type: '',
  prompt_id: '',
  prompt_version: '',
})

const jobTypeOptions = [
  'analyze',
  'generate',
  'document_process',
  'document_reprocess',
  'knowledge_process',
  'knowledge_reprocess',
]
const jobStatusOptions = [
  { value: 'active', label: '执行中（含排队/重试）' },
  { value: 'pending', label: '等待中' },
  { value: 'retrying', label: '重试中' },
  { value: 'running', label: '运行中' },
  { value: 'succeeded', label: '已成功' },
  { value: 'failed', label: '已失败' },
  { value: 'canceled', label: '已取消' },
]
const feedbackTypeOptions = [
  { value: 'useful', label: '有用' },
  { value: 'duplicate', label: '重复' },
  { value: 'missing_steps', label: '缺步骤' },
  { value: 'requirement_mismatch', label: '不符合需求' },
  { value: 'knowledge_missing', label: '知识缺失' },
]
const jobResourceOptions = [
  { value: 'task_id', label: '任务' },
  { value: 'document_id', label: '文档' },
  { value: 'knowledge_id', label: '知识' },
]

const currentTenantLabel = computed(() => {
  const tenant = tenants.value.find((item) => item.slug === currentSlug.value)
  return tenant ? `${tenant.name} (${tenant.slug})` : currentSlug.value || '-'
})
const metricsSummary = computed(() => metrics.value?.summary || {})
const modelMetrics = computed(() => metrics.value?.by_model || [])
const workflowMetrics = computed(() => metrics.value?.by_workflow || [])
const failureStages = computed(() => metrics.value?.failure_stages || [])
const jobStatuses = computed(() => metrics.value?.job_statuses || [])
const preflightChecks = computed(() => preflight.value?.checks || [])
const preflightJSON = computed(() => (preflight.value ? JSON.stringify(preflight.value, null, 2) : ''))
const feedbackByType = computed(() => feedbackSummary.value?.by_type || [])
const feedbackByPrompt = computed(() => feedbackSummary.value?.by_prompt || [])
const recentFeedback = computed(() => feedbackSummary.value?.recent || [])
const qualityPromptComparison = computed(() => qualityOverview.value?.prompt_comparison || [])
const qualityProfileComparison = computed(() => qualityOverview.value?.profile_comparison || [])
const qualityTrend = computed(() => qualityOverview.value?.feedback_trend || [])
const qualityReportHistory = computed(() => qualityOverview.value?.report_history || [])
const activeJobCount = computed(() => jobs.value.filter((job) => ['pending', 'retrying', 'running'].includes(job.status)).length)
const failedJobCount = computed(() => jobs.value.filter((job) => ['failed', 'canceled'].includes(job.status)).length)
const reviewedCaseCount = computed(() => Number(feedbackSummary.value?.reviewed_cases || 0))
const usefulCaseCount = computed(() => Number(feedbackSummary.value?.useful_cases || 0))
const issueCaseCount = computed(() => Number(feedbackSummary.value?.issue_cases || 0))
const usefulCaseRate = computed(() => reviewedCaseCount.value ? usefulCaseCount.value / reviewedCaseCount.value : 0)
const visibleJobs = computed(() => {
  if (jobFilters.status !== 'active') return jobs.value
  return jobs.value.filter((job) => ['pending', 'retrying', 'running'].includes(job.status))
})
const recentAnomalies = computed(() => {
  const jobRows = jobs.value
    .filter((job) => ['failed', 'canceled', 'retrying'].includes(job.status))
    .map((job) => ({
      key: `job-${job.id}`,
      kind: '运行',
      status: job.status === 'retrying' ? 'warn' : 'fail',
      title: `${jobTypeLabel(job.job_type)} · ${resourceLabel(job)}`,
      detail: compactJobError(job) || jobStatusLabel(job.status),
      time: job.updated_at || job.created_at,
      job,
    }))
  const checkRows = preflightChecks.value
    .filter((check) => ['warn', 'fail'].includes(check.status))
    .map((check, index) => ({
      key: `check-${check.name || index}`,
      kind: '环境',
      status: check.status,
      title: check.label || check.name || '环境检查',
      detail: check.message || '需要检查',
      time: preflight.value?.generated_at,
      check,
    }))
  return [...jobRows, ...checkRows]
    .sort((left, right) => new Date(right.time || 0) - new Date(left.time || 0))
    .slice(0, 8)
})

onMounted(() => {
  tenantStore.fetch().catch(() => {})
  loadActiveTab().catch(() => {})
})

watch(activeTab, () => {
  loadActiveTab().catch(() => {})
})

function buildJobParams() {
  const params = {}
  if (jobFilters.job_type) params.job_type = jobFilters.job_type
  if (jobFilters.status && jobFilters.status !== 'active') params.status = jobFilters.status
  if (jobFilters.resource_type && jobFilters.resource_id) {
    params[jobFilters.resource_type] = Number(jobFilters.resource_id)
  }
  return params
}

function buildMetricsParams() {
  const params = {}
  if (metricsRange.value?.[0]) params.from = metricsRange.value[0]
  if (metricsRange.value?.[1]) params.to = metricsRange.value[1]
  if (metricsFilters.provider) params.provider = metricsFilters.provider.trim()
  if (metricsFilters.model) params.model = metricsFilters.model.trim()
  if (metricsFilters.workflow_type) params.workflow_type = metricsFilters.workflow_type
  if (metricsFilters.task_id) params.task_id = Number(metricsFilters.task_id)
  return params
}

function buildFeedbackParams() {
  const params = {}
  if (qualityRange.value?.[0]) params.from = qualityRange.value[0]
  if (qualityRange.value?.[1]) params.to = qualityRange.value[1]
  if (qualityFilters.task_id) params.task_id = Number(qualityFilters.task_id)
  if (qualityFilters.feedback_type) params.feedback_type = qualityFilters.feedback_type
  if (qualityFilters.prompt_id) params.prompt_id = qualityFilters.prompt_id.trim()
  if (qualityFilters.prompt_version) params.prompt_version = qualityFilters.prompt_version.trim()
  return params
}

function buildQualityParams() {
  const params = buildFeedbackParams()
  delete params.feedback_type
  return params
}

async function loadJobs(useFilters = true) {
  jobsLoading.value = true
  try {
    jobs.value = await listJobs(useFilters ? buildJobParams() : {})
  } finally {
    jobsLoading.value = false
  }
}

async function loadWorkflows() {
  workflowsLoading.value = true
  try {
    workflows.value = await listWorkflows()
  } finally {
    workflowsLoading.value = false
  }
}

async function loadMetrics() {
  metricsLoading.value = true
  try {
    metrics.value = await getOpsMetrics(buildMetricsParams())
  } finally {
    metricsLoading.value = false
  }
}

async function loadPreflight() {
  preflightLoading.value = true
  try {
    preflight.value = await getOpsPreflight()
  } finally {
    preflightLoading.value = false
  }
}

async function loadFeedbackSummary() {
  feedbackLoading.value = true
  try {
    feedbackSummary.value = await getFeedbackSummary(buildFeedbackParams())
  } finally {
    feedbackLoading.value = false
  }
}

async function loadQualityOverview() {
  qualityLoading.value = true
  try {
    qualityOverview.value = await getQualityOverview(buildQualityParams())
  } finally {
    qualityLoading.value = false
  }
}

async function loadActiveTab(force = false) {
  const tab = activeTab.value
  if (tabLoaded[tab] && !force) return
  if (tab === 'overview') await Promise.all([loadMetrics(), loadPreflight(), loadFeedbackSummary(), loadJobs(false)])
  if (tab === 'runs') await Promise.all([loadJobs(), loadWorkflows()])
  if (tab === 'quality') await Promise.all([loadFeedbackSummary(), loadQualityOverview()])
  if (tab === 'models') await loadMetrics()
  if (tab === 'environment') await loadPreflight()
  tabLoaded[tab] = true
  lastRefreshedAt.value = new Date().toISOString()
}

async function refreshCurrentView() {
  await loadActiveTab(true)
}

function switchView(tab, setup) {
  setup?.()
  tabLoaded[tab] = false
  if (activeTab.value === tab) loadActiveTab(true).catch(() => {})
  else activeTab.value = tab
}

function resetJobFilters() {
  Object.assign(jobFilters, { job_type: '', status: '', resource_type: '', resource_id: '' })
  loadJobs().catch(() => {})
}

function resetMetricsFilters() {
  metricsRange.value = []
  Object.assign(metricsFilters, { provider: '', model: '', workflow_type: '', task_id: '' })
  loadMetrics().catch(() => {})
}

function resetQualityFilters() {
  qualityRange.value = []
  Object.assign(qualityFilters, { task_id: '', feedback_type: '', prompt_id: '', prompt_version: '' })
  Promise.all([loadFeedbackSummary(), loadQualityOverview()]).catch(() => {})
}

function resourceLabel(job) {
  if (job.task_id) return `任务 #${job.task_id}`
  if (job.document_id) return `文档 #${job.document_id}`
  if (job.knowledge_id) return `知识 #${job.knowledge_id}`
  return '-'
}

function workflowsForJob(job) {
  return workflows.value.filter((run) => run.job_id === job.id || run.id === job.workflow_run_id)
}

function canRetry(job) {
  return ['failed', 'canceled'].includes(job.status)
}

function canCancel(job) {
  return ['pending', 'retrying', 'running'].includes(job.status)
}

function canReplay(job) {
  return ['succeeded', 'failed', 'canceled'].includes(job.status)
}

async function confirmAction(job, action) {
  const labels = { retry: '重试', cancel: '取消', replay: '重放' }
  const operatorName = await ensureOperatorName()
  const reason = await promptInterventionReason(`${labels[action]} job #${job.id}`, action)
  await ElMessageBox.confirm(
    `${labels[action]} job #${job.id}？操作者：${operatorName}`,
    '确认操作',
    { type: action === 'cancel' ? 'warning' : 'info' },
  )
  return { reason }
}

async function ensureOperatorName() {
  const current = localStorage.getItem(OPERATOR_NAME_STORAGE_KEY) || ''
  if (current.trim()) {
    if (!localStorage.getItem(OPERATOR_ID_STORAGE_KEY)) {
      localStorage.setItem(OPERATOR_ID_STORAGE_KEY, operatorIDFromName(current))
    }
    return current.trim()
  }
  const { value } = await ElMessageBox.prompt('请输入本次操作人名称', '操作者', {
    confirmButtonText: '确认',
    cancelButtonText: '取消',
    inputPattern: /\S+/,
    inputErrorMessage: '操作者不能为空',
  })
  const name = String(value || '').trim()
  localStorage.setItem(OPERATOR_NAME_STORAGE_KEY, name)
  localStorage.setItem(OPERATOR_ID_STORAGE_KEY, operatorIDFromName(name))
  return name
}

async function promptInterventionReason(title, action) {
  const labels = { retry: '重试原因', cancel: '取消原因', replay: '重放原因' }
  const { value } = await ElMessageBox.prompt(`请填写${labels[action] || '操作原因'}：${title}`, '操作原因', {
    confirmButtonText: '确认',
    cancelButtonText: '取消',
    inputType: 'textarea',
    inputPattern: /\S+/,
    inputErrorMessage: '原因不能为空',
  })
  return String(value || '').trim()
}

function operatorIDFromName(name) {
  const normalized = name.trim().toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '')
  return `local:${normalized || 'operator'}`
}

async function runJobAction(job, action) {
  let payload
  try {
    payload = await confirmAction(job, action)
  } catch {
    return
  }
  actingJobId.value = job.id
  try {
    if (action === 'retry') await retryJob(job.id, payload)
    if (action === 'cancel') await cancelJob(job.id, payload)
    if (action === 'replay') await replayJob(job.id, payload)
    notifySuccess(`job #${job.id} ${action} 已提交`)
    tabLoaded.overview = false
    await Promise.all([loadJobs(), loadWorkflows()])
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    actingJobId.value = 0
  }
}

function openTask(jobOrFeedback) {
  const id = jobOrFeedback?.task_id
  if (id) router.push({ name: 'task-detail', params: { id } })
}

async function copyPreflightJSON() {
  if (!preflightJSON.value) return
  preflightCopying.value = true
  try {
    await navigator.clipboard.writeText(preflightJSON.value)
    notifySuccess('环境检查 JSON 已复制')
  } catch {
    await ElMessageBox.alert('复制失败，请展开原始 JSON 后手动复制。', '复制失败', { type: 'warning' })
  } finally {
    preflightCopying.value = false
  }
}

function feedbackTypeLabel(value) {
  return feedbackTypeOptions.find((item) => item.value === value)?.label || value || '-'
}

function checkStatusType(status) {
  if (status === 'pass') return 'success'
  if (status === 'warn') return 'warning'
  if (status === 'fail') return 'danger'
  return 'info'
}

function checkStatusLabel(status) {
  return { pass: '正常', warn: '警告', fail: '失败' }[status] || '未知'
}

function checkSuggestion(check) {
  if (check.status === 'pass') return '无需处理。'
  if (check.status === 'warn') return '确认该告警是否符合当前 Demo 配置；不符合时根据提示修正配置后重新诊断。'
  return '根据提示检查依赖服务或运行配置，修复后重新诊断。'
}

function feedbackNegativeRate(row) {
  if (!row?.total) return '0%'
  return formatPercent(Number(row.negative || 0) / Number(row.total))
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString()
}

function formatPercent(value) {
  return `${Math.round(Number(value || 0) * 100)}%`
}

function formatDuration(ms) {
  const value = Number(ms || 0)
  if (!value) return '-'
  if (value < 1000) return `${value} ms`
  return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)} s`
}

function formatElapsed(start, finish) {
  if (!start) return '-'
  const end = finish ? new Date(finish).getTime() : Date.now()
  const elapsed = end - new Date(start).getTime()
  return elapsed >= 0 ? formatDuration(elapsed) : '-'
}
</script>

<template>
  <section class="ops-workbench">
    <header class="page-header">
      <div>
        <h2>运维工作台</h2>
        <p class="muted">
          当前租户：<strong>{{ currentTenantLabel }}</strong>
          <span v-if="lastRefreshedAt"> · 更新于 {{ formatDate(lastRefreshedAt) }}</span>
        </p>
      </div>
      <el-button
        :icon="Refresh"
        :loading="tenantLoading || jobsLoading || workflowsLoading || metricsLoading || preflightLoading || feedbackLoading || qualityLoading"
        @click="refreshCurrentView"
      >刷新当前视图</el-button>
    </header>

    <el-tabs v-model="activeTab" class="ops-tabs">
      <el-tab-pane label="运行概览" name="overview">
        <div class="summary-strip" v-loading="metricsLoading || preflightLoading || feedbackLoading || jobsLoading">
          <button class="summary-item" type="button" @click="switchView('environment')">
            <span>环境状态</span>
            <strong :class="`tone-${preflight?.overall || 'unknown'}`">{{ checkStatusLabel(preflight?.overall) }}</strong>
            <small>{{ preflightChecks.filter((item) => item.status !== 'pass').length }} 项需关注</small>
          </button>
          <button class="summary-item" type="button" @click="switchView('runs', () => { jobFilters.status = 'active' })">
            <span>执行中</span>
            <strong>{{ formatNumber(activeJobCount) }}</strong>
            <small>排队、运行和重试</small>
          </button>
          <button class="summary-item" type="button" @click="switchView('runs', () => { jobFilters.status = 'failed' })">
            <span>近期失败</span>
            <strong :class="{ 'tone-fail': failedJobCount }">{{ formatNumber(failedJobCount) }}</strong>
            <small>失败或取消的运行</small>
          </button>
          <button class="summary-item" type="button" @click="switchView('models')">
            <span>工作流成功率</span>
            <strong>{{ formatPercent(metricsSummary.workflow_success_rate) }}</strong>
            <small>{{ formatNumber(metricsSummary.workflow_runs) }} 次运行</small>
          </button>
          <button class="summary-item" type="button" @click="switchView('models')">
            <span>平均耗时</span>
            <strong>{{ formatDuration(metricsSummary.average_workflow_ms) }}</strong>
            <small>模型 {{ formatDuration(metricsSummary.average_model_latency_ms) }}</small>
          </button>
          <button class="summary-item" type="button" @click="switchView('quality')">
            <span>问题用例</span>
            <strong :class="{ 'tone-warn': issueCaseCount }">{{ formatNumber(issueCaseCount) }}</strong>
            <small>{{ reviewedCaseCount }} 条已审核</small>
          </button>
        </div>

        <section class="content-section">
          <div class="section-heading">
            <div>
              <h3>最近异常</h3>
              <p class="muted">优先展示失败、重试和环境告警。</p>
            </div>
          </div>
          <el-table :data="recentAnomalies" stripe class="ops-table">
            <el-table-column prop="kind" label="来源" width="90" />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="checkStatusType(row.status)">{{ checkStatusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="title" label="对象" min-width="220" />
            <el-table-column prop="detail" label="问题" min-width="320" show-overflow-tooltip />
            <el-table-column label="时间" width="190">
              <template #default="{ row }">{{ formatDate(row.time) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="150" fixed="right">
              <template #default="{ row }">
                <el-button v-if="row.job?.task_id" link type="primary" @click="openTask(row.job)">查看任务</el-button>
                <el-button v-else-if="row.check" link type="primary" @click="switchView('environment')">查看检查</el-button>
                <span v-else class="muted">-</span>
              </template>
            </el-table-column>
            <template #empty>当前没有需要处理的异常</template>
          </el-table>
        </section>
      </el-tab-pane>

      <el-tab-pane label="运行记录" name="runs">
        <div class="filter-bar">
          <el-select v-model="jobFilters.job_type" clearable placeholder="操作类型" class="filter-control">
            <el-option v-for="item in jobTypeOptions" :key="item" :label="jobTypeLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="jobFilters.status" clearable placeholder="运行状态" class="filter-control wide">
            <el-option v-for="item in jobStatusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-select v-model="jobFilters.resource_type" clearable placeholder="业务对象" class="filter-control">
            <el-option v-for="item in jobResourceOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-input v-model="jobFilters.resource_id" clearable placeholder="对象 ID" class="id-input" />
          <el-button type="primary" :icon="Refresh" :loading="jobsLoading || workflowsLoading" @click="Promise.all([loadJobs(), loadWorkflows()])">查询</el-button>
          <el-button @click="resetJobFilters">重置</el-button>
        </div>

        <el-table :data="visibleJobs" v-loading="jobsLoading || workflowsLoading" stripe class="ops-table">
          <el-table-column type="expand" width="48">
            <template #default="{ row }">
              <div class="run-detail">
                <div class="detail-grid">
                  <div><span>Job ID</span><strong>#{{ row.id }}</strong></div>
                  <div><span>排队时间</span><strong>{{ formatDate(row.created_at) }}</strong></div>
                  <div><span>开始时间</span><strong>{{ formatDate(row.started_at) }}</strong></div>
                  <div><span>结束时间</span><strong>{{ formatDate(row.finished_at) }}</strong></div>
                </div>
                <el-table :data="workflowsForJob(row)" size="small" class="inner-table">
                  <el-table-column prop="id" label="Workflow" width="100" />
                  <el-table-column label="类型" min-width="150">
                    <template #default="{ row: run }">{{ jobTypeLabel(run.workflow_type) }}</template>
                  </el-table-column>
                  <el-table-column label="状态" width="110">
                    <template #default="{ row: run }">
                      <el-tag size="small" :type="jobStatusType(run.status)">{{ jobStatusLabel(run.status) }}</el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column label="开始" width="190">
                    <template #default="{ row: run }">{{ formatDate(run.started_at || run.created_at) }}</template>
                  </el-table-column>
                  <el-table-column label="耗时" width="120">
                    <template #default="{ row: run }">{{ formatElapsed(run.started_at || run.created_at, run.finished_at) }}</template>
                  </el-table-column>
                  <el-table-column prop="last_error" label="执行错误" min-width="280" show-overflow-tooltip />
                  <template #empty>该 Job 暂无关联 Workflow 记录</template>
                </el-table>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="id" label="ID" width="76" />
          <el-table-column label="操作类型" min-width="150">
            <template #default="{ row }">{{ jobTypeLabel(row.job_type) }}</template>
          </el-table-column>
          <el-table-column label="业务对象" min-width="150">
            <template #default="{ row }">
              <el-button v-if="row.task_id" link type="primary" @click="openTask(row)">{{ resourceLabel(row) }}</el-button>
              <span v-else>{{ resourceLabel(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="120">
            <template #default="{ row }">
              <el-tag size="small" :type="jobStatusType(row.status)">{{ jobStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="开始时间" width="190">
            <template #default="{ row }">{{ formatDate(row.started_at || row.run_after || row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="耗时" width="110">
            <template #default="{ row }">{{ formatElapsed(row.started_at, row.finished_at) }}</template>
          </el-table-column>
          <el-table-column label="重试" width="80">
            <template #default="{ row }">{{ row.retry_count }}/{{ row.max_retries }}</template>
          </el-table-column>
          <el-table-column label="错误摘要" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">{{ compactJobError(row) || '-' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <el-tooltip v-if="row.task_id" content="打开任务" placement="top">
                <el-button :icon="View" :aria-label="`打开任务 ${row.task_id}`" size="small" circle @click="openTask(row)" />
              </el-tooltip>
              <el-tooltip v-if="canRetry(row)" content="重试" placement="top">
                <el-button :icon="RefreshRight" :aria-label="`重试 job ${row.id}`" size="small" circle :loading="actingJobId === row.id" @click="runJobAction(row, 'retry')" />
              </el-tooltip>
              <el-tooltip v-if="canCancel(row)" content="取消" placement="top">
                <el-button :icon="Close" :aria-label="`取消 job ${row.id}`" size="small" circle :loading="actingJobId === row.id" @click="runJobAction(row, 'cancel')" />
              </el-tooltip>
              <el-tooltip v-if="canReplay(row)" content="重放" placement="top">
                <el-button :icon="CopyDocument" :aria-label="`重放 job ${row.id}`" size="small" circle :loading="actingJobId === row.id" @click="runJobAction(row, 'replay')" />
              </el-tooltip>
            </template>
          </el-table-column>
          <template #empty>暂无运行记录</template>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="质量与反馈" name="quality">
        <div class="filter-bar">
          <el-date-picker v-model="qualityRange" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" class="date-range" />
          <el-input v-model="qualityFilters.task_id" clearable placeholder="任务 ID" class="id-input" />
          <el-select v-model="qualityFilters.feedback_type" clearable placeholder="反馈类型" class="filter-control">
            <el-option v-for="item in feedbackTypeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-input v-model="qualityFilters.prompt_id" clearable placeholder="Prompt ID" class="filter-control" />
          <el-input v-model="qualityFilters.prompt_version" clearable placeholder="Prompt 版本" class="filter-control" />
          <el-button type="primary" :icon="Refresh" :loading="feedbackLoading || qualityLoading" @click="Promise.all([loadFeedbackSummary(), loadQualityOverview()])">查询</el-button>
          <el-button @click="resetQualityFilters">重置</el-button>
        </div>

        <div class="summary-strip compact" v-loading="feedbackLoading">
          <div class="summary-item static"><span>已审核用例</span><strong>{{ formatNumber(reviewedCaseCount) }}</strong><small>按最新反馈去重</small></div>
          <div class="summary-item static"><span>通过比例</span><strong>{{ formatPercent(usefulCaseRate) }}</strong><small>{{ usefulCaseCount }} 条最新反馈为有用</small></div>
          <div class="summary-item static"><span>问题用例</span><strong :class="{ 'tone-warn': issueCaseCount }">{{ formatNumber(issueCaseCount) }}</strong><small>最新反馈仍为问题</small></div>
          <div class="summary-item static"><span>反馈记录</span><strong>{{ formatNumber(feedbackSummary?.total) }}</strong><small>包含历史修改记录</small></div>
        </div>

        <div class="split-sections">
          <section class="content-section">
            <h3>问题类型分布</h3>
            <el-table :data="feedbackByType" v-loading="feedbackLoading" stripe class="ops-table">
              <el-table-column label="反馈类型" min-width="180">
                <template #default="{ row }">{{ feedbackTypeLabel(row.feedback_type) }}</template>
              </el-table-column>
              <el-table-column label="记录数" width="110">
                <template #default="{ row }">{{ formatNumber(row.count) }}</template>
              </el-table-column>
              <template #empty>暂无反馈类型数据</template>
            </el-table>
          </section>
          <section class="content-section">
            <h3>Prompt 反馈</h3>
            <el-table :data="feedbackByPrompt" v-loading="feedbackLoading" stripe class="ops-table">
              <el-table-column prop="prompt_id" label="Prompt" min-width="150" />
              <el-table-column prop="prompt_version" label="版本" width="110" />
              <el-table-column label="样本" width="90"><template #default="{ row }">{{ formatNumber(row.total) }}</template></el-table-column>
              <el-table-column label="问题率" width="100"><template #default="{ row }">{{ feedbackNegativeRate(row) }}</template></el-table-column>
              <template #empty>暂无 Prompt 反馈数据</template>
            </el-table>
          </section>
        </div>

        <section class="content-section">
          <h3>最近反馈</h3>
          <el-table :data="recentFeedback" v-loading="feedbackLoading" stripe class="ops-table">
            <el-table-column prop="id" label="ID" width="76" />
            <el-table-column label="任务" width="100">
              <template #default="{ row }"><el-button link type="primary" @click="openTask(row)">#{{ row.task_id }}</el-button></template>
            </el-table-column>
            <el-table-column label="类型" min-width="140"><template #default="{ row }">{{ feedbackTypeLabel(row.feedback_type) }}</template></el-table-column>
            <el-table-column prop="case_title" label="用例" min-width="240" show-overflow-tooltip />
            <el-table-column prop="note" label="说明" min-width="240" show-overflow-tooltip />
            <el-table-column label="时间" width="190"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
            <template #empty>暂无反馈记录</template>
          </el-table>
        </section>

        <el-collapse v-model="advancedQualitySections" class="advanced-analysis">
          <el-collapse-item title="高级分析：Prompt、生成配置、趋势与报告" name="advanced">
            <div class="split-sections">
              <section class="content-section">
                <h3>Prompt 对比</h3>
                <el-table :data="qualityPromptComparison" v-loading="qualityLoading" stripe class="ops-table">
                  <el-table-column prop="prompt_id" label="Prompt" min-width="150" />
                  <el-table-column prop="prompt_version" label="版本" width="110" />
                  <el-table-column label="样本" width="90"><template #default="{ row }">{{ formatNumber(row.total) }}</template></el-table-column>
                  <el-table-column label="有用" width="90"><template #default="{ row }">{{ formatNumber(row.useful) }}</template></el-table-column>
                  <el-table-column label="问题率" width="100"><template #default="{ row }">{{ feedbackNegativeRate(row) }}</template></el-table-column>
                </el-table>
              </section>
              <section class="content-section">
                <h3>生成配置对比</h3>
                <el-table :data="qualityProfileComparison" v-loading="qualityLoading" stripe class="ops-table">
                  <el-table-column prop="profile_id" label="Profile" min-width="190" />
                  <el-table-column prop="profile_version" label="版本" min-width="150" show-overflow-tooltip />
                  <el-table-column label="样本" width="90"><template #default="{ row }">{{ formatNumber(row.total) }}</template></el-table-column>
                  <el-table-column label="问题率" width="100"><template #default="{ row }">{{ feedbackNegativeRate(row) }}</template></el-table-column>
                </el-table>
              </section>
            </div>
            <div class="split-sections">
              <section class="content-section">
                <h3>反馈趋势</h3>
                <el-table :data="qualityTrend" v-loading="qualityLoading" stripe class="ops-table">
                  <el-table-column prop="date" label="日期" width="130" />
                  <el-table-column label="类型" min-width="170"><template #default="{ row }">{{ feedbackTypeLabel(row.feedback_type) }}</template></el-table-column>
                  <el-table-column label="数量" width="100"><template #default="{ row }">{{ formatNumber(row.count) }}</template></el-table-column>
                </el-table>
              </section>
              <section class="content-section">
                <h3>质量报告</h3>
                <el-table :data="qualityReportHistory" v-loading="qualityLoading" stripe class="ops-table">
                  <el-table-column prop="artifact_id" label="Artifact" width="96" />
                  <el-table-column prop="task_id" label="任务" width="86" />
                  <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
                  <el-table-column label="用例" width="90"><template #default="{ row }">{{ formatNumber(row.case_count) }}</template></el-table-column>
                  <el-table-column label="时间" width="190"><template #default="{ row }">{{ formatDate(row.created_at) }}</template></el-table-column>
                </el-table>
              </section>
            </div>
          </el-collapse-item>
        </el-collapse>
      </el-tab-pane>

      <el-tab-pane label="模型与稳定性" name="models">
        <div class="filter-bar">
          <el-date-picker v-model="metricsRange" type="daterange" value-format="YYYY-MM-DD" start-placeholder="开始日期" end-placeholder="结束日期" class="date-range" />
          <el-select v-model="metricsFilters.workflow_type" clearable placeholder="工作流" class="filter-control">
            <el-option v-for="item in jobTypeOptions" :key="item" :label="jobTypeLabel(item)" :value="item" />
          </el-select>
          <el-input v-model="metricsFilters.provider" clearable placeholder="Provider" class="filter-control" />
          <el-input v-model="metricsFilters.model" clearable placeholder="Model" class="filter-control" />
          <el-input v-model="metricsFilters.task_id" clearable placeholder="任务 ID" class="id-input" />
          <el-button type="primary" :icon="Refresh" :loading="metricsLoading" @click="loadMetrics">查询</el-button>
          <el-button @click="resetMetricsFilters">重置</el-button>
        </div>

        <div class="summary-strip compact" v-loading="metricsLoading">
          <div class="summary-item static"><span>模型调用</span><strong>{{ formatNumber(metricsSummary.model_calls) }}</strong><small>成功率 {{ formatPercent(metricsSummary.model_success_rate) }}</small></div>
          <div class="summary-item static"><span>计费 Token</span><strong>{{ formatNumber(metricsSummary.accounted_tokens) }}</strong><small>{{ formatNumber(metricsSummary.prompt_chars) }} prompt 字符</small></div>
          <div class="summary-item static"><span>Fallback</span><strong>{{ formatNumber(metricsSummary.fallbacks) }}</strong><small>{{ formatNumber(metricsSummary.rate_limits) }} 次限流</small></div>
          <div class="summary-item static"><span>保护信号</span><strong>{{ formatNumber((metricsSummary.circuit_open || 0) + (metricsSummary.budget_exceeded || 0)) }}</strong><small>熔断与预算耗尽</small></div>
        </div>

        <section class="content-section">
          <h3>模型调用</h3>
          <el-table :data="modelMetrics" v-loading="metricsLoading" stripe class="ops-table">
            <el-table-column prop="provider" label="Provider" min-width="120" />
            <el-table-column prop="model" label="Model" min-width="160" />
            <el-table-column label="调用" width="90"><template #default="{ row }">{{ formatNumber(row.calls) }}</template></el-table-column>
            <el-table-column label="成功率" width="100"><template #default="{ row }">{{ formatPercent(row.success_rate) }}</template></el-table-column>
            <el-table-column label="Token" min-width="120"><template #default="{ row }">{{ formatNumber(row.accounted_tokens) }}</template></el-table-column>
            <el-table-column label="平均延迟" width="120"><template #default="{ row }">{{ formatDuration(row.average_model_latency_ms) }}</template></el-table-column>
            <el-table-column label="异常信号" min-width="240">
              <template #default="{ row }">
                <el-tag v-if="row.fallbacks" size="small" type="warning" class="chip">{{ row.fallbacks }} fallback</el-tag>
                <el-tag v-if="row.rate_limits" size="small" type="danger" class="chip">{{ row.rate_limits }} rate limit</el-tag>
                <el-tag v-if="row.circuit_open" size="small" type="danger" class="chip">{{ row.circuit_open }} circuit</el-tag>
                <el-tag v-if="row.budget_exceeded" size="small" type="danger" class="chip">{{ row.budget_exceeded }} budget</el-tag>
                <span v-if="!row.fallbacks && !row.rate_limits && !row.circuit_open && !row.budget_exceeded" class="muted">-</span>
              </template>
            </el-table-column>
            <template #empty>暂无模型调用数据</template>
          </el-table>
        </section>

        <section class="content-section">
          <h3>工作流稳定性</h3>
          <el-table :data="workflowMetrics" v-loading="metricsLoading" stripe class="ops-table">
            <el-table-column label="工作流" min-width="160"><template #default="{ row }">{{ jobTypeLabel(row.workflow_type) }}</template></el-table-column>
            <el-table-column label="运行" width="90"><template #default="{ row }">{{ formatNumber(row.runs) }}</template></el-table-column>
            <el-table-column label="成功率" width="100"><template #default="{ row }">{{ formatPercent(row.success_rate) }}</template></el-table-column>
            <el-table-column label="模型调用" width="110"><template #default="{ row }">{{ formatNumber(row.model_calls) }}</template></el-table-column>
            <el-table-column label="失败" min-width="170"><template #default="{ row }">{{ formatNumber(row.failed_agents) }} agent · {{ formatNumber(row.failed_jobs) }} job</template></el-table-column>
            <el-table-column label="平均耗时" width="130"><template #default="{ row }">{{ formatDuration(row.average_duration_ms) }}</template></el-table-column>
            <template #empty>暂无工作流稳定性数据</template>
          </el-table>
        </section>

        <div class="split-sections">
          <section class="content-section">
            <h3>失败阶段</h3>
            <el-table :data="failureStages" v-loading="metricsLoading" stripe class="ops-table">
              <el-table-column prop="agent" label="Agent" min-width="120" />
              <el-table-column prop="stage" label="阶段" min-width="140" />
              <el-table-column label="失败" width="90"><template #default="{ row }">{{ formatNumber(row.failures) }}</template></el-table-column>
              <el-table-column prop="last_error" label="最近错误" min-width="240" show-overflow-tooltip />
              <template #empty>暂无失败阶段</template>
            </el-table>
          </section>
          <section class="content-section">
            <h3>Job 状态</h3>
            <el-table :data="jobStatuses" v-loading="metricsLoading" stripe class="ops-table">
              <el-table-column label="类型" min-width="150"><template #default="{ row }">{{ jobTypeLabel(row.type) }}</template></el-table-column>
              <el-table-column label="状态" width="120"><template #default="{ row }"><el-tag size="small" :type="jobStatusType(row.status)">{{ jobStatusLabel(row.status) }}</el-tag></template></el-table-column>
              <el-table-column label="数量" width="100"><template #default="{ row }">{{ formatNumber(row.count) }}</template></el-table-column>
              <template #empty>暂无 Job 状态数据</template>
            </el-table>
          </section>
        </div>
      </el-tab-pane>

      <el-tab-pane label="环境检查" name="environment">
        <div class="environment-head">
          <div>
            <el-tag :type="checkStatusType(preflight?.overall)" size="large">{{ checkStatusLabel(preflight?.overall) }}</el-tag>
            <span class="muted">诊断时间：{{ formatDate(preflight?.generated_at) }}</span>
          </div>
          <div>
            <el-button type="primary" :icon="Refresh" :loading="preflightLoading" @click="loadPreflight">重新诊断</el-button>
            <el-button :icon="CopyDocument" :loading="preflightCopying" :disabled="!preflight" @click="copyPreflightJSON">复制 JSON</el-button>
          </div>
        </div>

        <el-table :data="preflightChecks" v-loading="preflightLoading" stripe class="ops-table">
          <el-table-column type="expand" width="48">
            <template #default="{ row }">
              <div class="check-detail">
                <strong>建议</strong>
                <p>{{ checkSuggestion(row) }}</p>
                <strong>技术详情</strong>
                <pre>{{ JSON.stringify(row.metadata || {}, null, 2) }}</pre>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="label" label="检查项" min-width="220" />
          <el-table-column label="状态" width="110"><template #default="{ row }"><el-tag size="small" :type="checkStatusType(row.status)">{{ checkStatusLabel(row.status) }}</el-tag></template></el-table-column>
          <el-table-column prop="message" label="结果" min-width="420" show-overflow-tooltip />
          <template #empty>暂无环境检查数据</template>
        </el-table>

        <el-collapse v-if="preflight" class="raw-json">
          <el-collapse-item title="原始检查 JSON" name="raw-json">
            <pre>{{ preflightJSON }}</pre>
          </el-collapse-item>
        </el-collapse>
      </el-tab-pane>
    </el-tabs>
  </section>
</template>

<style scoped>
.ops-workbench {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-header,
.environment-head,
.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}
.page-header h2,
.content-section h3,
.section-heading h3 {
  margin: 0;
}
.page-header h2 {
  font-size: 20px;
}
.page-header p,
.section-heading p {
  margin: 6px 0 0;
}
.ops-tabs {
  background: #fff;
  border: 1px solid #e5e7eb;
  padding: 0 18px 18px;
}
.summary-strip {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  border: 1px solid #e5e7eb;
  background: #fff;
  margin-bottom: 18px;
}
.summary-strip.compact {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}
.summary-item {
  min-width: 0;
  min-height: 112px;
  padding: 16px;
  border: 0;
  border-right: 1px solid #e5e7eb;
  border-radius: 0;
  background: transparent;
  text-align: left;
  color: #111827;
}
button.summary-item {
  cursor: pointer;
}
button.summary-item:hover {
  background: #f8fafc;
}
.summary-item:last-child {
  border-right: 0;
}
.summary-item span,
.summary-item strong,
.summary-item small {
  display: block;
}
.summary-item span {
  color: #64748b;
  font-size: 12px;
}
.summary-item strong {
  margin-top: 8px;
  font-size: 24px;
  line-height: 1.2;
}
.summary-item small {
  margin-top: 6px;
  color: #64748b;
  font-size: 12px;
}
.filter-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.filter-control {
  width: 160px;
}
.filter-control.wide {
  width: 210px;
}
.id-input {
  width: 120px;
}
.date-range {
  width: 260px;
}
.content-section {
  min-width: 0;
  margin-top: 18px;
}
.content-section h3 {
  margin-bottom: 10px;
  font-size: 15px;
}
.split-sections {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px;
}
.ops-table {
  width: 100%;
}
.run-detail,
.check-detail {
  padding: 14px 24px 18px 64px;
  background: #f8fafc;
}
.detail-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  margin-bottom: 14px;
}
.detail-grid span,
.detail-grid strong {
  display: block;
}
.detail-grid span {
  color: #64748b;
  font-size: 12px;
  margin-bottom: 4px;
}
.inner-table {
  background: transparent;
}
.advanced-analysis,
.raw-json {
  margin-top: 18px;
}
.environment-head {
  align-items: center;
  margin-bottom: 16px;
}
.environment-head > div {
  display: flex;
  align-items: center;
  gap: 10px;
}
.check-detail p {
  margin: 6px 0 14px;
}
.check-detail pre,
.raw-json pre {
  margin: 8px 0 0;
  padding: 12px;
  overflow: auto;
  border: 1px solid #dbe3ec;
  background: #fff;
  color: #334155;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}
.muted {
  color: #64748b;
  font-size: 13px;
}
.chip {
  margin: 2px 4px 2px 0;
}
.tone-pass {
  color: #15803d;
}
.tone-warn {
  color: #b45309;
}
.tone-fail {
  color: #b91c1c;
}
.tone-unknown {
  color: #64748b;
}
</style>
