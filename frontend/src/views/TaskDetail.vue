<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { ElMessageBox } from 'element-plus'
import { Check, Download, EditPen, Refresh } from '@element-plus/icons-vue'
import StatusTag from '../components/StatusTag.vue'
import { listJobs } from '../api/jobs'
import { getTaskTrace } from '../api/tasks'
import { useTasksStore } from '../stores/tasks'
import { useTestCasesStore } from '../stores/testcases'
import { useKnowledgeStore } from '../stores/knowledge'
import { useKnowledgeSuggestionsStore } from '../stores/knowledgeSuggestions'
import { notifySuccess } from '../utils/error'
import { compactJobError, jobStatusLabel, jobStatusType, jobTypeLabel } from '../utils/jobs'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => Number(route.params.id))

const tasksStore = useTasksStore()
const casesStore = useTestCasesStore()
const knowledgeStore = useKnowledgeStore()
const suggestionStore = useKnowledgeSuggestionsStore()

const { current: task } = storeToRefs(tasksStore)
const { items: cases, loading: casesLoading, saving: casesSaving } = storeToRefs(casesStore)
const { items: knowledge } = storeToRefs(knowledgeStore)

const reviewForm = reactive({ products: [], modules: [] })
const polling = ref(null)
const generating = ref(false)
const retrying = ref(false)
const jobs = ref([])
const jobsLoading = ref(false)
const trace = ref(null)
const traceLoading = ref(false)
const editingCase = ref(null)
const editorVisible = ref(false)
const editorBuffer = ref('')
const feedbackVisible = ref(false)
const feedbackForm = reactive({
  candidate_type: 'module',
  candidate_name: '',
  source_case_id: 0,
  source_task_id: 0,
  source_case_title: '',
  note: '',
})

const productOptions = computed(() =>
  knowledge.value.filter((k) => k.type === 'product').map((k) => k.name),
)
const moduleOptions = computed(() =>
  knowledge.value.filter((k) => k.type === 'module').map((k) => k.name),
)

const canReview = computed(() =>
  task.value && ['awaiting_review', 'ready_to_generate'].includes(task.value.status),
)
const canGenerate = computed(() => task.value?.status === 'ready_to_generate')
const isPolling = computed(() => task.value && ['analyzing', 'generating'].includes(task.value.status))
const caseSectionCount = computed(() => cases.value.length)
const totalCaseCount = computed(() =>
  cases.value.reduce((sum, section) => sum + (section.cases?.length || 0), 0),
)
const caseOutputEmptyDescription = computed(() => {
  switch (task.value?.status) {
    case 'analyzing':
      return '正在分析影响范围'
    case 'awaiting_review':
      return '等待影响范围审核'
    case 'ready_to_generate':
      return '等待开始生成'
    case 'generating':
      return '正在生成测试用例'
    case 'failed':
      return '暂无已保存用例'
    case 'completed':
      return '暂无用例'
    default:
      return '暂无用例'
  }
})
const jobTimeline = computed(() => {
  if (!task.value) return []

  const rows = [
    {
      key: 'created',
      label: 'Created',
      status: 'succeeded',
      time: task.value.created_at,
    },
  ]

  for (const job of jobs.value) {
    rows.push({
      key: `job-${job.id}`,
      label: jobTypeLabel(job.job_type),
      status: job.status,
      time: job.started_at || job.run_after || job.created_at,
      retry: `${job.retry_count}/${job.max_retries}`,
      error: compactJobError(job),
      nextRun: job.status === 'retrying' ? job.run_after : '',
    })
  }

  if (['awaiting_review', 'ready_to_generate'].includes(task.value.status)) {
    rows.push({
      key: 'review',
      label: 'Review',
      status: task.value.status === 'awaiting_review' ? 'running' : 'succeeded',
      time: task.value.updated_at,
    })
  }
  if (['completed', 'failed'].includes(task.value.status)) {
    rows.push({
      key: 'final',
      label: task.value.status === 'completed' ? 'Completed' : 'Failed',
      status: task.value.status === 'completed' ? 'succeeded' : 'failed',
      time: task.value.updated_at,
      error: lastJobError.value,
    })
  }
  return rows
})
const lastJobError = computed(() => compactJobError(findLastJobWithError()))
const traceSummary = computed(() => {
  const value = trace.value || {}
  return {
    workflows: value.workflow_runs?.length || 0,
    steps: value.steps?.length || 0,
    agents: value.agent_runs?.length || 0,
    modelCalls: value.model_calls?.length || 0,
    retrievals: value.retrieval_runs?.length || 0,
    artifacts: value.artifacts?.length || 0,
    lastError: latestTraceError(value),
  }
})
const traceRuns = computed(() => trace.value?.workflow_runs || [])
const traceAgents = computed(() => trace.value?.agent_runs || [])
const traceModelCalls = computed(() => trace.value?.model_calls || [])
const traceRetrievals = computed(() => trace.value?.retrieval_runs || [])
const traceArtifacts = computed(() => trace.value?.artifacts || [])
const hasTraceData = computed(() =>
  Boolean(
    traceSummary.value.workflows ||
      traceSummary.value.steps ||
      traceSummary.value.agents ||
      traceSummary.value.modelCalls ||
      traceSummary.value.retrievals ||
      traceSummary.value.artifacts,
  ),
)

onMounted(async () => {
  await loadTask()
  knowledgeStore.fetch().catch(() => {})
})
onUnmounted(() => stopPolling())

watch(taskId, () => {
  stopPolling()
  casesStore.clear()
  loadTask()
})

watch(
  () => task.value?.status,
  (status) => {
    if (status === 'completed') {
      refreshCases().catch(() => {})
      stopPolling()
    } else if (status === 'failed') {
      refreshCases().catch(() => {})
      stopPolling()
    } else if (status === 'analyzing' || status === 'generating') {
      casesStore.clear()
      ensurePolling()
    } else {
      casesStore.clear()
      stopPolling()
    }
  },
)

async function loadTask() {
  try {
    const t = await tasksStore.load(taskId.value)
    loadJobs().catch(() => {})
    loadTrace().catch(() => {})
    reviewForm.products = [...(t.affected_products || [])]
    reviewForm.modules = [...(t.affected_modules || [])]
    if (['completed', 'failed'].includes(t.status)) {
      refreshCases().catch(() => {})
    } else {
      casesStore.clear()
    }
  } catch {
    /* 错误已弹窗 */
  }
}

async function loadJobs() {
  jobsLoading.value = true
  try {
    jobs.value = await listJobs({ task_id: taskId.value })
  } finally {
    jobsLoading.value = false
  }
}

async function loadTrace() {
  traceLoading.value = true
  try {
    trace.value = await getTaskTrace(taskId.value)
  } finally {
    traceLoading.value = false
  }
}

async function refreshCases() {
  await casesStore.fetch(taskId.value)
}

function ensurePolling() {
  if (polling.value) return
  polling.value = setInterval(() => {
    tasksStore.load(taskId.value).catch(() => {})
    loadJobs().catch(() => {})
    loadTrace().catch(() => {})
  }, 3000)
}
function stopPolling() {
  if (polling.value) {
    clearInterval(polling.value)
    polling.value = null
  }
}

async function submitReview() {
  try {
    await tasksStore.review(taskId.value, {
      affected_products: reviewForm.products,
      affected_modules: reviewForm.modules,
    })
    notifySuccess('影响范围已提交')
  } catch {
    /* 错误已弹窗 */
  }
}

async function startGenerate() {
  generating.value = true
  try {
    await tasksStore.generate(taskId.value)
    notifySuccess('生成已触发，正在等待结果...')
    ensurePolling()
  } catch {
    /* 错误已弹窗 */
  } finally {
    generating.value = false
  }
}

async function retryTask() {
  retrying.value = true
  try {
    const updated = await tasksStore.retry(taskId.value)
    if (updated.status === 'analyzing') {
      notifySuccess('已重新触发分析，正在等待结果...')
      ensurePolling()
    } else {
      notifySuccess('任务已回到 ready_to_generate；可点击"开始生成"重新触发。')
    }
  } catch {
    /* 错误已弹窗 */
  } finally {
    retrying.value = false
  }
}

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : '-'
}

function findLastJobWithError() {
  for (let i = jobs.value.length - 1; i >= 0; i -= 1) {
    if (jobs.value[i].last_error) return jobs.value[i]
  }
  return null
}

function latestTraceError(value) {
  const rows = [
    ...(value.workflow_runs || []),
    ...(value.steps || []),
    ...(value.agent_runs || []),
    ...(value.model_calls || []),
    ...(value.retrieval_runs || []),
  ]
    .filter((row) => row.last_error)
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0) - new Date(a.updated_at || a.created_at || 0))
  return compactTraceText(rows[0]?.last_error || '')
}

function compactTraceText(value) {
  if (!value) return ''
  return value.length > 160 ? `${value.slice(0, 157)}...` : value
}

function traceStatusLabel(status) {
  return {
    pending: 'pending',
    running: 'running',
    succeeded: 'succeeded',
    failed: 'failed',
    canceled: 'canceled',
  }[status] || status || '-'
}

function artifactLabel(artifact) {
  const payload = artifact.payload || {}
  const counts = []
  if (payload.section_count !== undefined) counts.push(`${payload.section_count} sections`)
  if (payload.case_count !== undefined) counts.push(`${payload.case_count} cases`)
  return counts.length ? counts.join(' · ') : artifact.artifact_type
}

function priorityLabel(id) {
  return { 1: 'Low', 2: 'Medium', 3: 'High', 4: 'Critical' }[id] || `P${id}`
}

function exportCases() {
  if (!cases.value.length) return
  const payload = {
    task_id: taskId.value,
    task_status: task.value?.status || '',
    exported_at: new Date().toISOString(),
    sections: cases.value,
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `caseagent-task-${taskId.value}-test-cases.json`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function openEditor(section) {
  editingCase.value = section
  editorBuffer.value = JSON.stringify(section.cases || [], null, 2)
  editorVisible.value = true
}

async function saveEditor() {
  let parsed
  try {
    parsed = JSON.parse(editorBuffer.value)
    if (!Array.isArray(parsed)) throw new Error('cases 必须是数组')
  } catch (err) {
    ElMessageBox.alert(`JSON 解析失败：${err.message}`)
    return
  }
  try {
    await casesStore.update(taskId.value, editingCase.value.id, {
      section: editingCase.value.section,
      cases: parsed,
    })
    notifySuccess('用例已保存')
    editorVisible.value = false
  } catch {
    /* 错误已弹窗 */
  }
}

async function submitSection(section) {
  try {
    await ElMessageBox.confirm(
      `提交 section「${section.section}」共 ${section.cases?.length || 0} 条用例？`,
      '确认',
      { type: 'info' },
    )
  } catch {
    return
  }
  try {
    await casesStore.submit(taskId.value, section.id)
    notifySuccess('已提交')
  } catch {
    /* 错误已弹窗 */
  }
}

function openKnowledgeFeedback(section, row) {
  const moduleName = (row.affected_modules || [])[0] || ''
  const productName = (row.affected_products || [])[0] || ''
  Object.assign(feedbackForm, {
    candidate_type: moduleName ? 'module' : 'product',
    candidate_name: moduleName || productName || '',
    source_case_id: section.id,
    source_task_id: taskId.value,
    source_case_title: row.title || section.section || '',
    note: '',
  })
  feedbackVisible.value = true
}

async function submitKnowledgeFeedback() {
  if (!feedbackForm.candidate_name.trim()) {
    ElMessageBox.alert('请输入候选名称')
    return
  }
  try {
    await suggestionStore.createManual({
      candidate_type: feedbackForm.candidate_type,
      candidate_name: feedbackForm.candidate_name.trim(),
      source_case_id: feedbackForm.source_case_id,
      source_task_id: feedbackForm.source_task_id,
      source_case_title: feedbackForm.source_case_title,
      note: feedbackForm.note.trim(),
    })
    feedbackVisible.value = false
    notifySuccess('知识缺失反馈已提交')
  } catch {
    /* 错误已弹窗 */
  }
}
</script>

<template>
  <section v-if="task" class="task-detail">
    <header class="page-header">
      <el-breadcrumb separator="/">
        <el-breadcrumb-item :to="{ name: 'projects' }">项目</el-breadcrumb-item>
        <el-breadcrumb-item
          :to="{ name: 'project-detail', params: { id: task.project_id } }"
        >项目 #{{ task.project_id }}</el-breadcrumb-item>
        <el-breadcrumb-item>任务 #{{ task.id }}</el-breadcrumb-item>
      </el-breadcrumb>
      <div class="task-meta">
        <StatusTag :status="task.status" />
        <span class="muted">文档数：{{ (task.document_ids || []).length }}</span>
        <span class="muted">更新：{{ formatDate(task.updated_at) }}</span>
        <el-button
          v-if="task.status === 'failed'"
          type="warning"
          size="small"
          :loading="retrying"
          @click="retryTask"
        >重试</el-button>
        <el-button
          v-if="task.status === 'failed'"
          size="small"
          @click="loadTask"
        >刷新状态</el-button>
        <el-tag v-if="isPolling" type="warning" size="small">轮询中（3s）</el-tag>
      </div>
      <p v-if="task.status === 'failed'" class="muted danger">
        任务失败。常见原因：模型 API 失败、子 Agent 全部失败且 DeepAgent fallback 也失败。
        点击"重试"会根据当前状态自动回到 analyze 或 ready_to_generate；若回到 ready_to_generate，
        请确认根因（后端日志关键字 <code>agent</code> / <code>document</code>）已修复后再"开始生成"。
      </p>
    </header>

    <el-card shadow="never" class="card">
      <template #header>
        <div class="card-header">
          <span>后台任务</span>
          <el-button size="small" :loading="jobsLoading" @click="loadJobs">刷新</el-button>
        </div>
      </template>
      <el-timeline class="job-timeline">
        <el-timeline-item
          v-for="item in jobTimeline"
          :key="item.key"
          :type="jobStatusType(item.status)"
          :timestamp="formatDate(item.time)"
          placement="top"
        >
          <div class="job-row">
            <div class="job-row-main">
              <span class="job-label">{{ item.label }}</span>
              <el-tag size="small" :type="jobStatusType(item.status)">
                {{ jobStatusLabel(item.status) }}
              </el-tag>
              <span v-if="item.retry" class="muted small">retry {{ item.retry }}</span>
              <span v-if="item.nextRun" class="muted small">next {{ formatDate(item.nextRun) }}</span>
            </div>
            <p v-if="item.error" class="muted danger job-error">{{ item.error }}</p>
          </div>
        </el-timeline-item>
      </el-timeline>
    </el-card>

    <el-card shadow="never" class="card trace-card" v-loading="traceLoading">
      <template #header>
        <div class="card-header">
          <span>Workflow Trace</span>
          <el-button size="small" :loading="traceLoading" @click="loadTrace">刷新</el-button>
        </div>
      </template>
      <div class="trace-summary-grid">
        <div class="trace-metric">
          <span class="metric-value">{{ traceSummary.workflows }}</span>
          <span class="metric-label">runs</span>
        </div>
        <div class="trace-metric">
          <span class="metric-value">{{ traceSummary.steps }}</span>
          <span class="metric-label">steps</span>
        </div>
        <div class="trace-metric">
          <span class="metric-value">{{ traceSummary.agents }}</span>
          <span class="metric-label">agents</span>
        </div>
        <div class="trace-metric">
          <span class="metric-value">{{ traceSummary.modelCalls }}</span>
          <span class="metric-label">models</span>
        </div>
        <div class="trace-metric">
          <span class="metric-value">{{ traceSummary.retrievals }}</span>
          <span class="metric-label">retrievals</span>
        </div>
        <div class="trace-metric">
          <span class="metric-value">{{ traceSummary.artifacts }}</span>
          <span class="metric-label">artifacts</span>
        </div>
      </div>
      <p v-if="traceSummary.lastError" class="muted danger trace-error">
        {{ traceSummary.lastError }}
      </p>
      <el-empty
        v-if="!hasTraceData && !traceLoading"
        description="暂无 workflow trace"
      />
      <div v-if="hasTraceData" class="trace-layout">
        <div class="trace-column">
          <h3>Runs</h3>
          <div v-for="run in traceRuns" :key="run.id" class="trace-row">
            <div class="trace-row-main">
              <span class="trace-title">{{ jobTypeLabel(run.workflow_type) }}</span>
              <el-tag size="small" :type="jobStatusType(run.status)">
                {{ traceStatusLabel(run.status) }}
              </el-tag>
            </div>
            <div class="muted small">
              #{{ run.id }} · {{ formatDate(run.started_at || run.created_at) }}
            </div>
            <p v-if="run.last_error" class="muted danger trace-error">
              {{ compactTraceText(run.last_error) }}
            </p>
          </div>
        </div>
        <div class="trace-column">
          <h3>Agents</h3>
          <div v-for="agent in traceAgents" :key="agent.id" class="trace-row">
            <div class="trace-row-main">
              <span class="trace-title">{{ agent.agent_name }}</span>
              <el-tag size="small" :type="jobStatusType(agent.status)">
                {{ traceStatusLabel(agent.status) }}
              </el-tag>
            </div>
            <div class="muted small">{{ agent.stage }} · {{ formatDate(agent.finished_at || agent.created_at) }}</div>
            <p v-if="agent.output_summary" class="trace-snippet">
              {{ compactTraceText(agent.output_summary) }}
            </p>
            <p v-if="agent.last_error" class="muted danger trace-error">
              {{ compactTraceText(agent.last_error) }}
            </p>
          </div>
          <el-empty v-if="!traceAgents.length && hasTraceData" description="暂无 agent runs" />
        </div>
        <div class="trace-column">
          <h3>Models</h3>
          <div v-for="call in traceModelCalls" :key="call.id" class="trace-row">
            <div class="trace-row-main">
              <span class="trace-title">{{ call.provider || '-' }} / {{ call.model || '-' }}</span>
              <el-tag size="small" :type="jobStatusType(call.status)">
                {{ traceStatusLabel(call.status) }}
              </el-tag>
            </div>
            <div class="muted small">
              {{ call.prompt_chars }} prompt · {{ call.response_chars }} response
            </div>
            <p v-if="call.last_error" class="muted danger trace-error">
              {{ compactTraceText(call.last_error) }}
            </p>
          </div>
          <el-empty v-if="!traceModelCalls.length && hasTraceData" description="暂无 model calls" />
        </div>
        <div class="trace-column">
          <h3>Retrievals</h3>
          <div v-for="retrieval in traceRetrievals" :key="retrieval.id" class="trace-row">
            <div class="trace-row-main">
              <span class="trace-title">{{ retrieval.retriever_type }}</span>
              <el-tag size="small" :type="jobStatusType(retrieval.status)">
                {{ traceStatusLabel(retrieval.status) }}
              </el-tag>
            </div>
            <div class="muted small">
              {{ retrieval.query_count }} queries · {{ retrieval.hit_count }} hits
            </div>
          </div>
          <el-empty v-if="!traceRetrievals.length && hasTraceData" description="暂无 retrieval runs" />
        </div>
        <div class="trace-column">
          <h3>Artifacts</h3>
          <div v-for="artifact in traceArtifacts" :key="artifact.id" class="trace-row">
            <div class="trace-row-main">
              <span class="trace-title">{{ artifact.name || artifact.artifact_type }}</span>
              <el-tag size="small" type="info">{{ artifact.artifact_type }}</el-tag>
            </div>
            <div class="muted small">
              {{ artifactLabel(artifact) }} · {{ formatDate(artifact.created_at) }}
            </div>
          </div>
          <el-empty v-if="!traceArtifacts.length && hasTraceData" description="暂无 artifacts" />
        </div>
      </div>
    </el-card>

    <el-card shadow="never" class="card">
      <template #header><span>影响范围审核</span></template>
      <el-form label-width="100px">
        <el-form-item label="受影响产品">
          <el-select
            v-model="reviewForm.products"
            multiple
            filterable
            allow-create
            :disabled="!canReview"
            style="width: 100%"
            placeholder="留空表示不限定"
          >
            <el-option v-for="p in productOptions" :key="p" :value="p" :label="p" />
          </el-select>
        </el-form-item>
        <el-form-item label="受影响模块">
          <el-select
            v-model="reviewForm.modules"
            multiple
            filterable
            allow-create
            :disabled="!canReview"
            style="width: 100%"
            placeholder="留空表示不限定"
          >
            <el-option v-for="m in moduleOptions" :key="m" :value="m" :label="m" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            :disabled="!canReview"
            @click="submitReview"
          >提交审核</el-button>
          <el-button
            type="success"
            :disabled="!canGenerate"
            :loading="generating"
            @click="startGenerate"
          >开始生成</el-button>
          <span v-if="!canReview && !canGenerate" class="muted small">
            当前状态 {{ task.status }} 不允许编辑影响范围或触发生成。
          </span>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="card case-output-card">
      <template #header>
        <div class="card-header">
          <span>测试用例输出</span>
          <div class="header-actions">
            <el-tag type="info" size="small">{{ caseSectionCount }} sections</el-tag>
            <el-tag type="success" size="small">{{ totalCaseCount }} cases</el-tag>
            <el-button
              :icon="Refresh"
              @click="refreshCases"
              :loading="casesLoading"
              :disabled="!['completed', 'failed'].includes(task.status)"
            >刷新</el-button>
            <el-button
              type="primary"
              :icon="Download"
              :disabled="!cases.length"
              @click="exportCases"
            >导出 JSON</el-button>
          </div>
        </div>
      </template>
      <el-empty
        v-if="!cases.length && !casesLoading"
        :description="caseOutputEmptyDescription"
      />
      <el-collapse v-else>
        <el-collapse-item
          v-for="section in cases"
          :key="section.id"
          :name="section.id"
        >
          <template #title>
            <div class="section-title">
              <span>{{ section.section }}</span>
              <el-tag size="small" type="info">{{ section.cases?.length || 0 }} cases</el-tag>
              <StatusTag :status="section.status" />
            </div>
          </template>

          <div class="section-actions">
            <el-button size="small" :icon="EditPen" @click="openEditor(section)">编辑 JSON</el-button>
            <el-button
              size="small"
              type="primary"
              :icon="Check"
              :disabled="section.status === 'submitted' || section.status === 'approved'"
              @click="submitSection(section)"
            >提交</el-button>
          </div>

          <el-table :data="section.cases || []" stripe size="small">
            <el-table-column type="expand" width="48">
              <template #default="{ row }">
                <div class="case-expand">
                  <div class="case-field">
                    <div class="field-label">前置条件</div>
                    <div class="field-body">{{ row.custom_preconds || '-' }}</div>
                  </div>
                  <div class="case-field">
                    <div class="field-label">步骤</div>
                    <el-table
                      :data="row.custom_steps_separated || []"
                      size="small"
                      border
                      class="steps-table"
                    >
                      <el-table-column label="#" width="56">
                        <template #default="{ $index }">{{ $index + 1 }}</template>
                      </el-table-column>
                      <el-table-column prop="content" label="操作" min-width="260" />
                      <el-table-column prop="expected" label="预期" min-width="260" />
                    </el-table>
                  </div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="title" label="标题" min-width="260" show-overflow-tooltip />
            <el-table-column label="优先级" width="100">
              <template #default="{ row }">{{ priorityLabel(row.priority_id) }}</template>
            </el-table-column>
            <el-table-column label="影响产品" min-width="140">
              <template #default="{ row }">
                <el-tag
                  v-for="p in row.affected_products || []"
                  :key="p"
                  size="small"
                  type="info"
                  class="chip"
                >{{ p }}</el-tag>
                <span v-if="!(row.affected_products || []).length" class="muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="影响模块" min-width="140">
              <template #default="{ row }">
                <el-tag
                  v-for="m in row.affected_modules || []"
                  :key="m"
                  size="small"
                  class="chip"
                >{{ m }}</el-tag>
                <span v-if="!(row.affected_modules || []).length" class="muted">-</span>
              </template>
            </el-table-column>
            <el-table-column prop="custom_preconds" label="前置条件" min-width="220" show-overflow-tooltip />
            <el-table-column label="步骤数" width="90">
              <template #default="{ row }">{{ (row.custom_steps_separated || []).length }}</template>
            </el-table-column>
            <el-table-column label="反馈" width="120" align="center">
              <template #default="{ row }">
                <el-button size="small" @click="openKnowledgeFeedback(section, row)">
                  知识缺失
                </el-button>
              </template>
            </el-table-column>
          </el-table>

          <details v-if="section.source_context" class="source-ctx">
            <summary>source_context 追溯（{{
              (section.source_context.knowledge_shipped_ids || []).length
            }} knowledge ids,
            {{ (section.source_context.document_hits || []).length }} doc hits）</summary>
            <pre>{{ JSON.stringify(section.source_context, null, 2) }}</pre>
          </details>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <el-dialog
      v-model="editorVisible"
      :title="`编辑 section: ${editingCase?.section || ''}`"
      width="780px"
      :close-on-click-modal="false"
    >
      <p class="muted small">
        直接编辑 JSON 数组（本 section 的全部 cases）。保存后整段替换；建议保留
        <code>title / priority_id / custom_preconds / custom_steps_separated</code> 四个字段。
      </p>
      <el-input
        v-model="editorBuffer"
        type="textarea"
        :rows="20"
        spellcheck="false"
        style="font-family: ui-monospace, Consolas, monospace"
      />
      <template #footer>
        <el-button @click="editorVisible = false">取消</el-button>
        <el-button type="primary" :loading="casesSaving" @click="saveEditor">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="feedbackVisible"
      title="反馈知识缺失"
      width="520px"
      :close-on-click-modal="false"
    >
      <el-form label-width="100px">
        <el-form-item label="类型">
          <el-radio-group v-model="feedbackForm.candidate_type">
            <el-radio value="product">product</el-radio>
            <el-radio value="module">module</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="候选名称">
          <el-input v-model="feedbackForm.candidate_name" maxlength="120" show-word-limit />
        </el-form-item>
        <el-form-item label="来源用例">
          <el-input v-model="feedbackForm.source_case_title" disabled />
        </el-form-item>
        <el-form-item label="备注">
          <el-input
            v-model="feedbackForm.note"
            type="textarea"
            :rows="4"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="feedbackVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="suggestionStore.saving"
          @click="submitKnowledgeFeedback"
        >提交</el-button>
      </template>
    </el-dialog>
  </section>
  <el-empty v-else description="加载中..." />
</template>

<style scoped>
.task-detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.page-header {
  background: #fff;
  border-radius: 8px;
  padding: 16px 24px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.task-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
}
.muted {
  color: #909399;
  font-size: 13px;
}
.danger {
  color: #f56c6c;
  margin-top: 12px;
}
.small {
  font-size: 12px;
}
.card {
  border-radius: 8px;
}
.trace-card {
  overflow: hidden;
}
.trace-summary-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 14px;
}
.trace-metric {
  min-width: 0;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 10px 12px;
  background: #fafafa;
}
.metric-value {
  display: block;
  color: #303133;
  font-size: 20px;
  font-weight: 700;
  line-height: 1.2;
}
.metric-label {
  display: block;
  margin-top: 2px;
  color: #909399;
  font-size: 12px;
}
.trace-layout {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
}
.trace-column {
  min-width: 0;
}
.trace-column h3 {
  margin: 0 0 8px;
  color: #606266;
  font-size: 13px;
  font-weight: 600;
}
.trace-row {
  min-width: 0;
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 10px;
  margin-bottom: 8px;
  background: #fff;
}
.trace-row-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}
.trace-title {
  min-width: 0;
  overflow: hidden;
  color: #303133;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trace-snippet {
  margin: 6px 0 0;
  color: #606266;
  font-size: 12px;
  line-height: 1.5;
  word-break: break-word;
}
.trace-error {
  margin-bottom: 8px;
  word-break: break-word;
}
.job-timeline {
  padding: 4px 0 0;
}
.job-row {
  min-width: 0;
}
.job-row-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.job-label {
  color: #303133;
  font-weight: 600;
}
.job-error {
  margin: 6px 0 0;
  word-break: break-word;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}
.section-actions {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}
.chip {
  margin-right: 4px;
}
.source-ctx {
  margin-top: 12px;
  background: #fafafa;
  padding: 12px;
  border-radius: 4px;
  font-size: 12px;
}
.source-ctx pre {
  white-space: pre-wrap;
  word-break: break-all;
  margin: 8px 0 0;
  max-height: 320px;
  overflow: auto;
}
.case-expand {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px 16px 12px 48px;
}
.case-field {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}
.field-label {
  color: #606266;
  font-size: 13px;
  font-weight: 500;
}
.field-body {
  color: #303133;
  line-height: 1.6;
  word-break: break-word;
}
.steps-table {
  width: 100%;
}
@media (max-width: 1200px) {
  .trace-summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .trace-layout {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 720px) {
  .task-meta,
  .card-header,
  .header-actions {
    align-items: flex-start;
    flex-direction: column;
  }
  .trace-summary-grid,
  .trace-layout {
    grid-template-columns: 1fr;
  }
}
</style>
