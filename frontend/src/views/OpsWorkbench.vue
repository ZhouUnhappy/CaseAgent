<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { Close, CopyDocument, Refresh, RefreshRight, View } from '@element-plus/icons-vue'
import { cancelJob, listJobs, replayJob, retryJob } from '../api/jobs'
import { getOpsMetrics } from '../api/ops'
import { listWorkflows } from '../api/workflows'
import { useTenantStore } from '../stores/tenant'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel } from '../utils/jobs'

const router = useRouter()
const tenantStore = useTenantStore()
const { activeItems: tenants, currentSlug, loading: tenantLoading } = storeToRefs(tenantStore)

const activeTab = ref('metrics')
const jobs = ref([])
const workflows = ref([])
const metrics = ref(null)
const jobsLoading = ref(false)
const workflowsLoading = ref(false)
const metricsLoading = ref(false)
const actingJobId = ref(0)
const metricsRange = ref([])

const jobFilters = reactive({
  tenant: currentSlug.value,
  job_type: '',
  status: '',
  resource_type: '',
  resource_id: '',
})

const workflowFilters = reactive({
  tenant: currentSlug.value,
  workflow_type: '',
  status: '',
  resource_type: '',
  resource_id: '',
  job_id: '',
})

const metricsFilters = reactive({
  tenant: currentSlug.value,
  provider: '',
  model: '',
  workflow_type: '',
  task_id: '',
})

const jobTypeOptions = [
  'analyze',
  'generate',
  'document_process',
  'document_reprocess',
  'knowledge_process',
  'knowledge_reprocess',
]
const jobStatusOptions = ['pending', 'retrying', 'running', 'succeeded', 'failed', 'canceled']
const workflowStatusOptions = ['pending', 'running', 'succeeded', 'failed', 'canceled']
const jobResourceOptions = [
  { value: 'task_id', label: 'Task' },
  { value: 'document_id', label: 'Document' },
  { value: 'knowledge_id', label: 'Knowledge' },
]
const workflowResourceOptions = [
  { value: 'task', label: 'Task' },
  { value: 'document', label: 'Document' },
  { value: 'knowledge', label: 'Knowledge' },
]

const metricsSummary = computed(() => metrics.value?.summary || {})
const modelMetrics = computed(() => metrics.value?.by_model || [])
const workflowMetrics = computed(() => metrics.value?.by_workflow || [])
const failureStages = computed(() => metrics.value?.failure_stages || [])
const jobStatuses = computed(() => metrics.value?.job_statuses || [])

const currentTenantLabel = computed(() => {
  const tenant = tenants.value.find((item) => item.slug === currentSlug.value)
  return tenant ? `${tenant.name} (${tenant.slug})` : currentSlug.value || '-'
})

onMounted(() => {
  tenantStore.fetch().catch(() => {})
  refreshAll()
})

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function applyTenant(slug) {
  tenantStore.setCurrent(slug)
  jobFilters.tenant = slug
  workflowFilters.tenant = slug
  metricsFilters.tenant = slug
  refreshAll()
}

function buildJobParams() {
  const params = {}
  if (jobFilters.job_type) params.job_type = jobFilters.job_type
  if (jobFilters.status) params.status = jobFilters.status
  if (jobFilters.resource_type && jobFilters.resource_id) {
    params[jobFilters.resource_type] = Number(jobFilters.resource_id)
  }
  return params
}

function buildWorkflowParams() {
  const params = {}
  if (workflowFilters.workflow_type) params.workflow_type = workflowFilters.workflow_type
  if (workflowFilters.status) params.status = workflowFilters.status
  if (workflowFilters.resource_type) params.resource_type = workflowFilters.resource_type
  if (workflowFilters.resource_id) params.resource_id = Number(workflowFilters.resource_id)
  if (workflowFilters.job_id) params.job_id = Number(workflowFilters.job_id)
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

async function loadJobs() {
  jobsLoading.value = true
  try {
    jobs.value = await listJobs(buildJobParams())
  } finally {
    jobsLoading.value = false
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

async function loadWorkflows() {
  workflowsLoading.value = true
  try {
    workflows.value = await listWorkflows(buildWorkflowParams())
  } finally {
    workflowsLoading.value = false
  }
}

function refreshAll() {
  loadMetrics().catch(() => {})
  loadJobs().catch(() => {})
  loadWorkflows().catch(() => {})
}

function resetJobFilters() {
  Object.assign(jobFilters, {
    tenant: currentSlug.value,
    job_type: '',
    status: '',
    resource_type: '',
    resource_id: '',
  })
  loadJobs().catch(() => {})
}

function resetWorkflowFilters() {
  Object.assign(workflowFilters, {
    tenant: currentSlug.value,
    workflow_type: '',
    status: '',
    resource_type: '',
    resource_id: '',
    job_id: '',
  })
  loadWorkflows().catch(() => {})
}

function resetMetricsFilters() {
  metricsRange.value = []
  Object.assign(metricsFilters, {
    tenant: currentSlug.value,
    provider: '',
    model: '',
    workflow_type: '',
    task_id: '',
  })
  loadMetrics().catch(() => {})
}

function resourceLabel(job) {
  if (job.task_id) return `task #${job.task_id}`
  if (job.document_id) return `document #${job.document_id}`
  if (job.knowledge_id) return `knowledge #${job.knowledge_id}`
  return '-'
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
  await ElMessageBox.confirm(`${labels[action]} job #${job.id}？`, '确认', { type: action === 'cancel' ? 'warning' : 'info' })
}

async function runJobAction(job, action) {
  try {
    await confirmAction(job, action)
  } catch {
    return
  }
  actingJobId.value = job.id
  try {
    if (action === 'retry') await retryJob(job.id)
    if (action === 'cancel') await cancelJob(job.id)
    if (action === 'replay') await replayJob(job.id)
    notifySuccess(`job #${job.id} ${action} 已提交`)
    refreshAll()
  } catch {
    /* api/client.js 已弹错 */
  } finally {
    actingJobId.value = 0
  }
}

function openTask(job) {
  if (!job.task_id) return
  router.push({ name: 'task-detail', params: { id: job.task_id } })
}

function workflowResourceLabel(run) {
  return `${run.resource_type || '-'} #${run.resource_id || '-'}`
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
</script>

<template>
  <section class="ops-workbench">
    <header class="page-header">
      <div>
        <h2>运维工作台</h2>
        <div class="tenant-line">
          <span class="muted">Tenant</span>
          <el-select
            :model-value="currentSlug"
            :loading="tenantLoading"
            size="small"
            class="tenant-select"
            @change="applyTenant"
          >
            <el-option
              v-for="tenant in tenants"
              :key="tenant.slug"
              :label="`${tenant.name} (${tenant.slug})`"
              :value="tenant.slug"
            />
          </el-select>
          <span class="muted small">{{ currentTenantLabel }}</span>
        </div>
      </div>
      <el-button :icon="Refresh" :loading="jobsLoading || workflowsLoading" @click="refreshAll">刷新</el-button>
    </header>

    <el-tabs v-model="activeTab" class="ops-tabs">
      <el-tab-pane label="Cost & Stability" name="metrics">
        <div class="filter-bar">
          <el-date-picker
            v-model="metricsRange"
            type="daterange"
            value-format="YYYY-MM-DD"
            start-placeholder="from"
            end-placeholder="to"
            class="date-range"
          />
          <el-select v-model="metricsFilters.workflow_type" clearable placeholder="workflow_type" class="filter-control">
            <el-option v-for="item in jobTypeOptions" :key="item" :label="jobTypeLabel(item)" :value="item" />
          </el-select>
          <el-input v-model="metricsFilters.provider" clearable placeholder="provider" class="filter-control" />
          <el-input v-model="metricsFilters.model" clearable placeholder="model" class="filter-control" />
          <el-input v-model="metricsFilters.task_id" clearable placeholder="task id" class="id-input" />
          <el-button type="primary" :icon="Refresh" :loading="metricsLoading" @click="loadMetrics">查询</el-button>
          <el-button @click="resetMetricsFilters">重置</el-button>
        </div>

        <div class="metric-grid" v-loading="metricsLoading">
          <div class="metric-panel">
            <span class="metric-label">Accounted tokens</span>
            <strong>{{ formatNumber(metricsSummary.accounted_tokens) }}</strong>
            <span class="metric-foot">{{ formatNumber(metricsSummary.prompt_chars) }} prompt chars</span>
          </div>
          <div class="metric-panel">
            <span class="metric-label">Model success</span>
            <strong>{{ formatPercent(metricsSummary.model_success_rate) }}</strong>
            <span class="metric-foot">{{ formatNumber(metricsSummary.model_calls) }} calls</span>
          </div>
          <div class="metric-panel">
            <span class="metric-label">Workflow success</span>
            <strong>{{ formatPercent(metricsSummary.workflow_success_rate) }}</strong>
            <span class="metric-foot">{{ formatNumber(metricsSummary.workflow_runs) }} runs</span>
          </div>
          <div class="metric-panel">
            <span class="metric-label">Fallbacks</span>
            <strong>{{ formatNumber(metricsSummary.fallbacks) }}</strong>
            <span class="metric-foot">{{ formatNumber(metricsSummary.rate_limits) }} rate limits</span>
          </div>
          <div class="metric-panel">
            <span class="metric-label">Guardrails</span>
            <strong>{{ formatNumber((metricsSummary.circuit_open || 0) + (metricsSummary.budget_exceeded || 0)) }}</strong>
            <span class="metric-foot">
              {{ formatNumber(metricsSummary.circuit_open) }} circuit · {{ formatNumber(metricsSummary.budget_exceeded) }} budget
            </span>
          </div>
          <div class="metric-panel">
            <span class="metric-label">Avg duration</span>
            <strong>{{ formatDuration(metricsSummary.average_workflow_ms) }}</strong>
            <span class="metric-foot">{{ formatDuration(metricsSummary.average_model_latency_ms) }} model</span>
          </div>
        </div>

        <div class="metrics-section">
          <h3>Model Cost</h3>
          <el-table :data="modelMetrics" v-loading="metricsLoading" stripe class="ops-table">
            <el-table-column prop="provider" label="Provider" min-width="120" />
            <el-table-column prop="model" label="Model" min-width="150" />
            <el-table-column label="Calls" width="92">
              <template #default="{ row }">{{ formatNumber(row.calls) }}</template>
            </el-table-column>
            <el-table-column label="Success" width="100">
              <template #default="{ row }">{{ formatPercent(row.success_rate) }}</template>
            </el-table-column>
            <el-table-column label="Tokens" min-width="120">
              <template #default="{ row }">{{ formatNumber(row.accounted_tokens) }}</template>
            </el-table-column>
            <el-table-column label="Chars" min-width="140">
              <template #default="{ row }">{{ formatNumber(row.prompt_chars + row.response_chars) }}</template>
            </el-table-column>
            <el-table-column label="Signals" min-width="190">
              <template #default="{ row }">
                <el-tag v-if="row.fallbacks" size="small" type="warning" class="chip">{{ row.fallbacks }} fallback</el-tag>
                <el-tag v-if="row.rate_limits" size="small" type="danger" class="chip">{{ row.rate_limits }} rate</el-tag>
                <el-tag v-if="row.circuit_open" size="small" type="danger" class="chip">{{ row.circuit_open }} circuit</el-tag>
                <el-tag v-if="row.budget_exceeded" size="small" type="danger" class="chip">{{ row.budget_exceeded }} budget</el-tag>
                <span v-if="!row.fallbacks && !row.rate_limits && !row.circuit_open && !row.budget_exceeded" class="muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="Avg Latency" width="120">
              <template #default="{ row }">{{ formatDuration(row.average_model_latency_ms) }}</template>
            </el-table-column>
            <template #empty>暂无 model cost 数据</template>
          </el-table>
        </div>

        <div class="metrics-section">
          <h3>Workflow Stability</h3>
          <el-table :data="workflowMetrics" v-loading="metricsLoading" stripe class="ops-table">
            <el-table-column prop="workflow_type" label="Workflow" min-width="150">
              <template #default="{ row }">{{ jobTypeLabel(row.workflow_type) }}</template>
            </el-table-column>
            <el-table-column label="Runs" width="90">
              <template #default="{ row }">{{ formatNumber(row.runs) }}</template>
            </el-table-column>
            <el-table-column label="Success" width="100">
              <template #default="{ row }">{{ formatPercent(row.success_rate) }}</template>
            </el-table-column>
            <el-table-column label="Model Calls" width="120">
              <template #default="{ row }">{{ formatNumber(row.model_calls) }}</template>
            </el-table-column>
            <el-table-column label="Tokens" min-width="120">
              <template #default="{ row }">{{ formatNumber(row.accounted_tokens) }}</template>
            </el-table-column>
            <el-table-column label="Failures" min-width="150">
              <template #default="{ row }">
                {{ formatNumber(row.failed_agents) }} agents · {{ formatNumber(row.failed_jobs) }} jobs
              </template>
            </el-table-column>
            <el-table-column label="Avg Duration" width="130">
              <template #default="{ row }">{{ formatDuration(row.average_duration_ms) }}</template>
            </el-table-column>
            <template #empty>暂无 workflow stability 数据</template>
          </el-table>
        </div>

        <div class="metrics-split">
          <div class="metrics-section">
            <h3>Failure Stages</h3>
            <el-table :data="failureStages" v-loading="metricsLoading" stripe class="ops-table">
              <el-table-column prop="agent" label="Agent" min-width="120" />
              <el-table-column prop="stage" label="Stage" min-width="140" />
              <el-table-column label="Failures" width="100">
                <template #default="{ row }">{{ formatNumber(row.failures) }}</template>
              </el-table-column>
              <el-table-column prop="last_error" label="Last Error" min-width="220" show-overflow-tooltip />
              <template #empty>暂无失败 stage</template>
            </el-table>
          </div>
          <div class="metrics-section">
            <h3>Job Statuses</h3>
            <el-table :data="jobStatuses" v-loading="metricsLoading" stripe class="ops-table">
              <el-table-column label="Type" min-width="150">
                <template #default="{ row }">{{ jobTypeLabel(row.type) }}</template>
              </el-table-column>
              <el-table-column label="Status" min-width="120">
                <template #default="{ row }">
                  <el-tag size="small" :type="jobStatusType(row.status)">{{ jobStatusLabel(row.status) }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="Count" width="100">
                <template #default="{ row }">{{ formatNumber(row.count) }}</template>
              </el-table-column>
              <template #empty>暂无 job 状态数据</template>
            </el-table>
          </div>
        </div>
      </el-tab-pane>

      <el-tab-pane label="Jobs" name="jobs">
        <div class="filter-bar">
          <el-select v-model="jobFilters.job_type" clearable placeholder="job_type" class="filter-control">
            <el-option v-for="item in jobTypeOptions" :key="item" :label="jobTypeLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="jobFilters.status" clearable placeholder="status" class="filter-control">
            <el-option v-for="item in jobStatusOptions" :key="item" :label="jobStatusLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="jobFilters.resource_type" clearable placeholder="resource" class="filter-control">
            <el-option v-for="item in jobResourceOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-input v-model="jobFilters.resource_id" clearable placeholder="resource id" class="id-input" />
          <el-button type="primary" :icon="Refresh" :loading="jobsLoading" @click="loadJobs">查询</el-button>
          <el-button @click="resetJobFilters">重置</el-button>
        </div>

        <el-table :data="jobs" v-loading="jobsLoading" stripe class="ops-table">
          <el-table-column prop="id" label="ID" width="76" />
          <el-table-column label="Type" min-width="150">
            <template #default="{ row }">{{ jobTypeLabel(row.job_type) }}</template>
          </el-table-column>
          <el-table-column label="Resource" min-width="150">
            <template #default="{ row }">{{ resourceLabel(row) }}</template>
          </el-table-column>
          <el-table-column label="Status" width="130">
            <template #default="{ row }">
              <el-tag size="small" :type="jobStatusType(row.status)">{{ jobStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Retry" width="92">
            <template #default="{ row }">{{ row.retry_count }}/{{ row.max_retries }}</template>
          </el-table-column>
          <el-table-column label="Run After" min-width="180">
            <template #default="{ row }">{{ formatDate(row.run_after) }}</template>
          </el-table-column>
          <el-table-column label="Updated" min-width="180">
            <template #default="{ row }">{{ formatDate(row.updated_at) }}</template>
          </el-table-column>
          <el-table-column label="Error" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ compactJobError(row) || '-' }}</template>
          </el-table-column>
          <el-table-column label="Actions" width="220" fixed="right">
            <template #default="{ row }">
              <el-tooltip content="打开任务" placement="top">
                <el-button :icon="View" size="small" circle :disabled="!row.task_id" @click="openTask(row)" />
              </el-tooltip>
              <el-tooltip content="重试" placement="top">
                <el-button
                  :icon="RefreshRight"
                  size="small"
                  circle
                  :loading="actingJobId === row.id"
                  :disabled="!canRetry(row)"
                  @click="runJobAction(row, 'retry')"
                />
              </el-tooltip>
              <el-tooltip content="取消" placement="top">
                <el-button
                  :icon="Close"
                  size="small"
                  circle
                  :loading="actingJobId === row.id"
                  :disabled="!canCancel(row)"
                  @click="runJobAction(row, 'cancel')"
                />
              </el-tooltip>
              <el-tooltip content="重放" placement="top">
                <el-button
                  :icon="CopyDocument"
                  size="small"
                  circle
                  :loading="actingJobId === row.id"
                  :disabled="!canReplay(row)"
                  @click="runJobAction(row, 'replay')"
                />
              </el-tooltip>
            </template>
          </el-table-column>
          <template #empty>暂无 jobs</template>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="Workflows" name="workflows">
        <div class="filter-bar">
          <el-select v-model="workflowFilters.workflow_type" clearable placeholder="workflow_type" class="filter-control">
            <el-option v-for="item in jobTypeOptions" :key="item" :label="jobTypeLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="workflowFilters.status" clearable placeholder="status" class="filter-control">
            <el-option v-for="item in workflowStatusOptions" :key="item" :label="jobStatusLabel(item)" :value="item" />
          </el-select>
          <el-select v-model="workflowFilters.resource_type" clearable placeholder="resource" class="filter-control">
            <el-option v-for="item in workflowResourceOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-input v-model="workflowFilters.resource_id" clearable placeholder="resource id" class="id-input" />
          <el-input v-model="workflowFilters.job_id" clearable placeholder="job id" class="id-input" />
          <el-button type="primary" :icon="Refresh" :loading="workflowsLoading" @click="loadWorkflows">查询</el-button>
          <el-button @click="resetWorkflowFilters">重置</el-button>
        </div>

        <el-table :data="workflows" v-loading="workflowsLoading" stripe class="ops-table">
          <el-table-column prop="id" label="ID" width="76" />
          <el-table-column label="Type" min-width="150">
            <template #default="{ row }">{{ jobTypeLabel(row.workflow_type) }}</template>
          </el-table-column>
          <el-table-column label="Resource" min-width="150">
            <template #default="{ row }">{{ workflowResourceLabel(row) }}</template>
          </el-table-column>
          <el-table-column label="Job" width="90">
            <template #default="{ row }">{{ row.job_id || '-' }}</template>
          </el-table-column>
          <el-table-column label="Status" width="130">
            <template #default="{ row }">
              <el-tag size="small" :type="jobStatusType(row.status)">{{ jobStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Started" min-width="180">
            <template #default="{ row }">{{ formatDate(row.started_at || row.created_at) }}</template>
          </el-table-column>
          <el-table-column label="Finished" min-width="180">
            <template #default="{ row }">{{ formatDate(row.finished_at) }}</template>
          </el-table-column>
          <el-table-column label="Error" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">{{ row.last_error || '-' }}</template>
          </el-table-column>
          <template #empty>暂无 workflow runs</template>
        </el-table>
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

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}

.page-header h2 {
  margin: 0 0 8px;
  font-size: 20px;
  font-weight: 700;
}

.tenant-line,
.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.tenant-select {
  width: 220px;
}

.ops-tabs {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 12px 14px 16px;
}

.filter-bar {
  padding: 4px 0 14px;
}

.filter-control {
  width: 170px;
}

.id-input {
  width: 140px;
}

.date-range {
  width: 260px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}

.metric-panel {
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 12px;
  background: #f8fafc;
}

.metric-panel strong {
  display: block;
  margin-top: 4px;
  color: #111827;
  font-size: 22px;
  line-height: 1.2;
}

.metric-label,
.metric-foot {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.metric-foot {
  margin-top: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.metrics-section {
  min-width: 0;
  margin-top: 16px;
}

.metrics-section h3 {
  margin: 0 0 10px;
  color: #334155;
  font-size: 14px;
  font-weight: 700;
}

.metrics-split {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.ops-table {
  width: 100%;
}

.chip {
  margin-right: 4px;
}

.muted {
  color: #64748b;
}

.small {
  font-size: 12px;
}

@media (max-width: 1280px) {
  .metric-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 860px) {
  .metric-grid,
  .metrics-split {
    grid-template-columns: 1fr;
  }

  .date-range,
  .filter-control,
  .id-input {
    width: 100%;
  }
}
</style>
