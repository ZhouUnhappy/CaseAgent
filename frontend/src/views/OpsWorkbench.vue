<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { Close, CopyDocument, Refresh, RefreshRight, View } from '@element-plus/icons-vue'
import { cancelJob, listJobs, replayJob, retryJob } from '../api/jobs'
import { listWorkflows } from '../api/workflows'
import { useTenantStore } from '../stores/tenant'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel } from '../utils/jobs'

const router = useRouter()
const tenantStore = useTenantStore()
const { items: tenants, currentSlug, loading: tenantLoading } = storeToRefs(tenantStore)

const activeTab = ref('jobs')
const jobs = ref([])
const workflows = ref([])
const jobsLoading = ref(false)
const workflowsLoading = ref(false)
const actingJobId = ref(0)

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
  window.location.reload()
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

async function loadJobs() {
  jobsLoading.value = true
  try {
    jobs.value = await listJobs(buildJobParams())
  } finally {
    jobsLoading.value = false
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

.ops-table {
  width: 100%;
}

.muted {
  color: #64748b;
}

.small {
  font-size: 12px;
}
</style>
